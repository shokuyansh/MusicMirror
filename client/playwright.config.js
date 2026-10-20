import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 45000,
  expect: { timeout: 10000 },
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  use: {
    baseURL: 'http://127.0.0.1:5173',
    browserName: 'chromium',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    launchOptions: { args: ['--autoplay-policy=no-user-gesture-required'] },
  },
  webServer: [
    {
      command: 'go run ./cmd/musicmirror',
      cwd: '../server',
      url: 'http://127.0.0.1:3001/healthz',
      reuseExistingServer: !process.env.CI,
      env: {
        APP_ENV: 'development',
        PORT: '3001',
        ALLOWED_ORIGINS: 'http://127.0.0.1:5173,http://localhost:5173',
        DATABASE_URL: '',
        SUPABASE_URL: '',
        SUPABASE_SECRET_KEY: '',
        DATA_DIR: 'data/e2e',
      },
    },
    {
      command: 'npm run dev -- --host 127.0.0.1',
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: !process.env.CI,
    },
  ],
})
