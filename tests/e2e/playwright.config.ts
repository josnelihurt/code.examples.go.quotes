import path from 'node:path';
import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

// The Go port's full-stack e2e configuration (ADR 0008). The frontend
// submodule's own playwright.fullstack.config.ts boots the .NET Release DLLs
// and is unloaded here — this repository has no dotnet, so this config drives
// the SAME feature suite (frontend/e2e, unchanged) against the two Go API
// processes scripts/e2e.sh starts before handing over (build, throwaway
// PostgreSQL, authapi + quotesapi on loopback ports with E2E_SIGNING_KEY).
// Only the Vite dev server is a webServer here: the APIs are already up, and
// their proxy targets are wired below exactly as the .NET job wired them.
//
// Transport selection (the wiring finding): the SPA picks its API version in
// src/api/client.ts — sessionStorage key "apiVersion", set through the UI
// switcher, defaulting to VITE_DEFAULT_API_VERSION (v1 when unset). The
// webServer below bakes v3 into that default because this backend serves
// only the v3 transport (ADR 0002); the scenarios that pin v0/v1/v2
// explicitly still cannot run against it: they are excluded below by name
// and file. What remains exercises the v3 journeys end to end — sign-in
// (authapi), the switching-transports v3 journey (random quote, catalog,
// publish through quotesapi + PostgreSQL), and sign-out — everything the
// frontend asks of this backend happens on v3.
// This config lives outside the frontend package (the Go repository's own
// e2e wiring), so playwright-bdd's featuresRoot — used verbatim, not
// resolved against the config file's directory — must be pinned to the
// submodule's e2e root explicitly, absolute, to keep it stable from any
// cwd. __dirname because playwright loads this file as CommonJS (there is
// no package.json out here to mark it a module).
const featuresRoot = path.resolve(__dirname, '../../frontend/e2e');

const testDir = defineBddConfig({
  features: `${featuresRoot}/features/**/*.feature`,
  steps: `${featuresRoot}/steps/**/*.ts`,
  featuresRoot,
  // The submodule's .gitignore already covers this generated directory name
  // (.features-gen-full); it is regenerated wholesale on every bddgen run.
  outputDir: '../../frontend/.features-gen-full',
});

const AUTH_PORT = process.env.E2E_AUTH_PORT ?? '5801';
const QUOTES_PORT = process.env.E2E_QUOTES_PORT ?? '5802';
const VITE_PORT = process.env.E2E_VITE_PORT ?? '5803';

const AUTH_HTTP = `http://127.0.0.1:${AUTH_PORT}`;
const QUOTES_HTTP = `http://127.0.0.1:${QUOTES_PORT}`;
const VITE_HTTP = `http://127.0.0.1:${VITE_PORT}`;

export default defineConfig({
  testDir,
  reporter: process.env.CI ? [['html', { open: 'never' }], ['list']] : 'html',
  use: { baseURL: VITE_HTTP, trace: 'on-first-retry' },
  projects: [{ name: 'chromium', use: devices['Desktop Chrome'] }],
  // The catalog lives in the throwaway PostgreSQL scripts/e2e.sh started,
  // shared by every scenario of the run, and browsing scenarios assert exact
  // page counts over the seeded catalog — one worker, like the .NET job.
  workers: 1,
  // The transport-pinned features this v3-only backend cannot serve:
  // browsing-quotes and publishing-quotes assert the v1 default and a v0
  // scenario, so they are out by file; reading-quotes and
  // switching-transports are in, minus the scenarios that pin v0/v2 or
  // assert the v1 default by name.
  testIgnore: /browsing-quotes\.feature|publishing-quotes\.feature/,
  grepInvert: /A random quote is displayed|The v0 transport serves the quote|The v2 transport serves the whole journey/,
  webServer: [
    {
      // --host pins IPv4 loopback: Vite binds ::1 only by default, which
      // leaves the 127.0.0.1 readiness probe refused even though the server
      // is up. The proxy targets are the Go API processes scripts/e2e.sh
      // already started.
      command: `pnpm run dev --host 127.0.0.1 --port ${VITE_PORT} --strictPort`,
      cwd: '../../frontend',
      env: {
        AUTH_API_HTTP: AUTH_HTTP,
        QUOTES_API_HTTP: QUOTES_HTTP,
        VITE_DEFAULT_API_VERSION: 'v3',
      },
      url: VITE_HTTP,
      reuseExistingServer: !process.env.CI,
    },
  ],
});
