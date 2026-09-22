import { test, expect, type Page } from '@playwright/test';

// Browser contract tests use isolated API fixtures; they do not create real connections.
const user = { id: '00000000-0000-0000-0000-000000000001', email: 'test@example.com', name: 'Test Admin', roles: ['admin'], permissions: ['*:*'] };

test.beforeEach(async ({ page }) => {
  await page.addInitScript(profile => localStorage.setItem('pepa_user', JSON.stringify(profile)), user);
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname;
    let body: unknown = {};
    if (path === '/api/v1/auth/me') {
      body = { user, roles: ['admin'], permissions: ['*:*'], enabled_plugins: [], connection_types: ['docker', 'secret'], get_started_completed: true };
    } else if (path.includes('/bootstrap')) {
      body = { needed: false, in_progress: false };
    } else if (path === '/api/v1/connections' && route.request().method() === 'POST') {
      body = { ...route.request().postDataJSON(), id: 'fixture-connection', status: 'disconnected' };
    } else if (path === '/api/v1/connections') {
      body = { connections: [], total: 0, page: 1, per_page: 200, total_pages: 0 };
    } else if (path === '/api/v1/connections/credential-status') {
      body = { statuses: [] };
    } else if (path === '/api/v1/plugins') {
      body = { plugins: [] };
    } else if (path.includes('notifications')) {
      body = { notifications: [], unread_count: 0 };
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
});

async function openConnectionForm(page: Page, type: 'Docker' | 'Vault / Secrets') {
  await page.goto('/connections');
  await page.getByRole('button', { name: /Add Connection/i }).first().click();
  await page.getByRole('button', { name: type === 'Docker' ? /Docker host for container management/ : /Vault \/ Secrets/ }).click();
  await expect(page.getByRole('heading', { name: `Add ${type} Connection` })).toBeVisible();
  await page.getByPlaceholder('e.g., Production Cluster').fill('Connection regression fixture');
}

async function submitConnection(page: Page) {
  const request = page.waitForRequest(req => new URL(req.url()).pathname === '/api/v1/connections' && req.method() === 'POST');
  await page.getByRole('button', { name: 'Create Connection', exact: true }).click();
  return (await request).postDataJSON() as { config: Record<string, string>; fallback_to_admin: boolean };
}

for (const allowFallback of [false, true]) {
  test(`Docker fallback controls submit ${allowFallback}`, async ({ page }) => {
    await openConnectionForm(page, 'Docker');
    const dockerControl = page.locator('#docker_admin_fallback');
    const commonControl = page.locator('#fallback_to_admin');
    await expect(dockerControl).toBeChecked();
    await expect(commonControl).toBeChecked();
    await dockerControl.uncheck();
    await expect(commonControl).not.toBeChecked();
    if (allowFallback) {
      await commonControl.check();
      await expect(dockerControl).toBeChecked();
    }
    const payload = await submitConnection(page);
    expect(payload.fallback_to_admin).toBe(allowFallback);
    expect(payload.config).not.toHaveProperty('admin_credential_fallback');
    expect(payload.config.host_type).toBe('local');
  });
}

test('Docker mode switches use the submitted host field', async ({ page }) => {
  await openConnectionForm(page, 'Docker');
  await expect(page.getByText('Host Address *', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: /TCP/, exact: false }).click();
  await page.getByPlaceholder('tcp://docker.example.com:2376').fill('tcp://old.example.com:2376');
  await page.getByRole('button', { name: /Local/, exact: false }).click();
  const payload = await submitConnection(page);
  expect(payload.config.host).toBe('unix:///var/run/docker.sock');
  expect(payload.config.host_type).toBe('local');
  expect(payload.config).not.toHaveProperty('host_address');
});

test('default built-in Vault mode is submitted explicitly', async ({ page }) => {
  await openConnectionForm(page, 'Vault / Secrets');
  const payload = await submitConnection(page);
  expect(payload.config.backend_mode).toBe('builtin');
});

test('SSH Docker requires a verified public host key', async ({ page }) => {
  await openConnectionForm(page, 'Docker');
  await page.getByRole('button', { name: /SSH/, exact: false }).click();
  await expect(page.locator('#docker_ssh_host_key')).toBeVisible();
  await expect(page.locator('#docker_ssh_host_key')).toHaveAttribute('required', '');
});
