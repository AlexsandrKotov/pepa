import { test, expect } from '@playwright/test';
import * as process from 'node:process';

test.describe('Services Golden Journey', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test.beforeEach(async ({ page }) => {
    // Login before each test
    const email = process.env.E2E_EMAIL;
    const password = process.env.E2E_PASSWORD;
    expect(email, 'Set E2E_EMAIL to an authorized test account').toBeTruthy();
    expect(password, 'Set E2E_PASSWORD for the test account').toBeTruthy();
    await page.goto('/login');
    const emailInput = page.locator('#email');
    await emailInput.waitFor({ state: 'visible', timeout: 15000 });
    await emailInput.fill(email!);
    await page.locator('#password').fill(password!);
    await page.click('button[type="submit"]');
    try {
      await expect(page).toHaveURL(url => ['/', '/dashboard', '/services'].includes(url.pathname));
    } finally {
      const passwordInput = page.locator('#password');
      if (await passwordInput.isVisible()) await passwordInput.fill('');
    }
  });

  test('view services list', async ({ page }) => {
    const loaded = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/services' && response.request().method() === 'GET');
    await page.goto('/services');
    const response = await loaded;
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(Array.isArray(data.items)).toBeTruthy();

    // Verify page loaded
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/service/i, {
      timeout: 10000,
    });

    // Verify at least one service card or table row exists (may be empty for new installs)
    if (data.items.length === 0) {
      // Empty state is acceptable for fresh installs
      await expect(page.getByText('No services yet', { exact: true })).toBeVisible();
    } else {
      await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 10000 });
    }
  });

  test('navigate to service detail', async ({ page }) => {
    const loaded = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/services' && response.request().method() === 'GET');
    await page.goto('/services');
    const response = await loaded;
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(Array.isArray(data.items)).toBeTruthy();
    expect(data.items.length, 'The live test instance needs an existing service for detail navigation').toBeGreaterThan(0);

    // Click on first service if exists
    const firstService = page.locator('table tbody tr').first();
    await expect(firstService).toBeVisible({ timeout: 10000 });
    const name = await firstService.locator('td').first().innerText();
    await firstService.locator('td').first().click();
    await expect(page).toHaveURL(url => url.pathname === '/services' && !!url.searchParams.get('id'));
    await expect(page.getByRole('heading', { level: 1, name, exact: true })).toBeVisible();
  });
});
