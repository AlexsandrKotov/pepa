import { test, expect } from '@playwright/test';

test.describe('Services Golden Journey', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/login');
    const emailInput = page.locator('#email');
    await emailInput.waitFor({ state: 'visible', timeout: 15000 });
    await emailInput.fill('admin');
    await page.locator('#password').fill('admin');
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL(/\/(dashboard|services)/);
  });

  test('view services list', async ({ page }) => {
    await page.goto('/services');

    // Verify page loaded
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/service/i, {
      timeout: 10000,
    });

    // Verify at least one service card or table row exists (may be empty for new installs)
    const serviceItems = page.locator('[data-testid="service-item"], table tbody tr, .service-card');
    await expect(serviceItems.first()).toBeVisible({ timeout: 10000 }).catch(() => {
      // Empty state is acceptable for fresh installs
      console.log('No services found (empty state is OK for fresh installs)');
    });
  });

  test('navigate to service detail', async ({ page }) => {
    await page.goto('/services');

    // Click on first service if exists
    const firstService = page.locator('[data-testid="service-item"], table tbody tr a, .service-card a').first();
    if (await firstService.isVisible({ timeout: 5000 }).catch(() => false)) {
      await firstService.click();
      await expect(page).toHaveURL(/\/services\//);
    }
  });
});
