import { setupServer } from 'msw/node'

/**
 * MSW server with no default handlers — each test registers exactly what it
 * needs via `server.use(...)`. Unhandled requests fail the test (see setup.ts).
 */
export const server = setupServer()

/**
 * The app fetches relative URLs; under jsdom MSW resolves both the request and
 * relative handler paths against the document location, so handlers can be
 * registered with the same relative path the app uses.
 */
export function api(path: string): string {
  return path
}
