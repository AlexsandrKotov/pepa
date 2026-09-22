import { test, expect } from '@playwright/test';
import * as process from 'node:process';

test.describe('Authentication Golden Journey', () => {
  test('login with configured test credentials', async ({ page }) => {
    const email = process.env.E2E_EMAIL;
    const password = process.env.E2E_PASSWORD;
    expect(email, 'Set E2E_EMAIL to an authorized test account').toBeTruthy();
    expect(password, 'Set E2E_PASSWORD for the test account').toBeTruthy();
    await page.goto('/login');

    // Wait for the login form to be visible (page may load bootstrap status first)
    const emailInput = page.locator('#email');
    await emailInput.waitFor({ state: 'visible', timeout: 15000 });

    // Fill login form
    await emailInput.fill(email!);
    await page.locator('#password').fill(password!);
    await page.click('button[type="submit"]');

    // Wait for redirect to dashboard
    try {
      await expect(page).toHaveURL(url => ['/', '/dashboard', '/services'].includes(url.pathname));
    } finally {
      const passwordInput = page.locator('#password');
      if (await passwordInput.isVisible()) await passwordInput.fill('');
    }

    // Verify user is logged in — check for user menu or avatar
    const response = await page.request.get('/api/v1/auth/me');
    expect(response.ok()).toBeTruthy();
    const { user } = await response.json();
    expect(user.email).toBe(email);
    expect(user.name).toBeTruthy();
    await expect(page.getByRole('button', { name: user.name })).toBeVisible({ timeout: 10000 });
  });

  test('redirect to login when not authenticated', async ({ page }) => {
    await page.goto('/services');
    await expect(page).toHaveURL(/\/login/);
  });
});
