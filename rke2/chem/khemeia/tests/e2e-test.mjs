import { chromium } from 'playwright';

const BASE_URL = 'https://khemeia.net';
const SCREENSHOT_DIR = '/tmp/khemeia-screenshots';

async function run() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  // Collect ALL console messages and errors
  const logs = [];
  page.on('console', msg => {
    logs.push(`[${msg.type()}] ${msg.text()}`);
  });
  page.on('pageerror', err => {
    logs.push(`[PAGE_ERROR] ${err.message}`);
  });
  page.on('requestfailed', req => {
    logs.push(`[REQ_FAILED] ${req.url()} - ${req.failure()?.errorText}`);
  });

  try {
    // 1. Load the site
    console.log('--- Loading khemeia.net ---');
    await page.goto(BASE_URL, { waitUntil: 'networkidle', timeout: 30000 });
    await page.screenshot({ path: `${SCREENSHOT_DIR}/01-initial-load.png`, fullPage: true });
    console.log('Page title:', await page.title());

    // Check if we see the splash page or the app
    const hasSplash = await page.$('.splash');
    const hasApp = await page.$('.app');
    console.log('Splash visible:', !!hasSplash);
    console.log('App visible:', !!hasApp);

    // If splash page, we need to auth. Check if there's a sign-in button
    if (hasSplash) {
      console.log('--- Splash page detected, checking auth ---');
      const signInBtn = await page.$('.splash-signin');
      if (signInBtn) {
        console.log('Sign-in button found. Cannot proceed without auth.');
        console.log('Will test the unauthenticated state only.');
      }
    }

    // If app is loaded (already authenticated), test the viewer
    if (hasApp) {
      console.log('--- App loaded, testing viewer ---');
      await page.screenshot({ path: `${SCREENSHOT_DIR}/02-app-loaded.png`, fullPage: true });

      // Wait for Molstar to initialize
      await page.waitForTimeout(3000);
      await page.screenshot({ path: `${SCREENSHOT_DIR}/03-after-3s.png`, fullPage: true });

      // Check if viewer container exists and has content
      const viewerContainer = await page.$('.viewer-container');
      console.log('Viewer container:', !!viewerContainer);

      // Check for Molstar canvas
      const canvas = await page.$('.viewer-container canvas');
      console.log('Molstar canvas:', !!canvas);

      // Check the Analysis tab
      const analysisTab = await page.$('text=Analysis');
      if (analysisTab) {
        console.log('--- Clicking Analysis tab ---');
        // Tab switching uses keys 1/2/3, press '2' for analysis
        await page.keyboard.press('2');
        await page.waitForTimeout(1000);
        await page.screenshot({ path: `${SCREENSHOT_DIR}/04-analysis-tab.png`, fullPage: true });
      }

      // Look for docking job results
      const jobRows = await page.$$('.job-row');
      console.log('Job rows found:', jobRows.length);

      // Check for results table
      const resultRows = await page.$$('.results-table tbody tr');
      console.log('Result rows found:', resultRows.length);

      // If there are results, try clicking "View" on the first one
      const viewBtn = await page.$('.view-btn');
      if (viewBtn) {
        console.log('--- Clicking View on first result ---');
        await viewBtn.click();
        await page.waitForTimeout(5000);
        await page.screenshot({ path: `${SCREENSHOT_DIR}/05-after-view-click.png`, fullPage: true });

        // Check if pocket analysis loaded
        const pocketSection = await page.$('text=Binding Pocket');
        console.log('Pocket section visible:', !!pocketSection);

        // Check if interaction map loaded
        const interactionMap = await page.$('.interaction-network');
        console.log('Interaction map visible:', !!interactionMap);

        // Wait more and screenshot again
        await page.waitForTimeout(3000);
        await page.screenshot({ path: `${SCREENSHOT_DIR}/06-after-8s.png`, fullPage: true });
      }
    }

  } catch (err) {
    console.error('Test error:', err.message);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/error.png`, fullPage: true });
  }

  // Dump all console logs
  console.log('\n--- Browser Console Output ---');
  for (const log of logs) {
    console.log(log);
  }

  await browser.close();
}

// Ensure screenshot dir exists
import { mkdirSync } from 'fs';
mkdirSync(SCREENSHOT_DIR, { recursive: true });

run().catch(err => {
  console.error('Fatal:', err);
  process.exit(1);
});
