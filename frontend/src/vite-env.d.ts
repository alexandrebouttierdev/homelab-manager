/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Release version, set by scripts/release/build.sh. */
  readonly VITE_APP_VERSION?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
