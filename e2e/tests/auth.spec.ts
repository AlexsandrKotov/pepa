import { test, expect } from '@playwright/test';

test.describe('Authentication Golden Journey', () => {
  test('login with default admin credentials', async ({ page }) => {
    await page.goto('/login');

    // Fill login form
    await page.fill('input[name="username"], input[type="text"]', 'admin');
    await page.fill('input[name="password"], input[type="password"]', 'admin');
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
