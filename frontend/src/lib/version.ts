/**
 * Version shown in the interface: the release version injected at build time
 * by scripts/release/build.sh (VITE_APP_VERSION), "dev" for a local build.
 */
export const APP_VERSION: string = import.meta.env.VITE_APP_VERSION || 'dev'
