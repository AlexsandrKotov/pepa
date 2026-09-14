import { test, expect } from '@playwright/test';

test.describe('Authentication Golden Journey', () => {
  test('login with default admin credentials', async ({ page }) => {
    await page.goto('/login');

    // Wait for the login form to be visible (page may load bootstrap status first)
    const emailInput = page.locator('#email');
    await emailInput.waitFor({ state: 'visible', timeout: 15000 });

    // Fill login form
    await emailInput.fill('admin');
    await page.locator('#password').fill('admin');
    await page.click('button[type="submit"]');

    // Wait for redirect to dashboard
    await expect(page).toHaveURL(/\/(dashboard|services)/);

    // Verify user is logged in — check for user menu or avatar
    await expect(page.locator('[data-testid="user-menu"], .user-avatar, [aria-label="User menu"]')).toBeVisible({
      timeout: 10000,
    });
  });

  test('redirect to login when not authenticated', async ({ page }) => {
    await page.goto('/services');
    await expect(page).toHaveURL(/\/login/);
  });
});
