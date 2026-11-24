type QrCodeProps = {
  /** SVG path of the dark modules (lib/qr.ts). */
  path: string
  viewBox: string
  label: string
}

export function QrCode({ path, viewBox, label }: QrCodeProps) {
  return (
    <svg
      viewBox={viewBox}
      role="img"
      aria-label={label}
      shapeRendering="crispEdges"
      className="h-44 w-44 shrink-0 rounded-md border border-line"
    >
      {/* Scanners need dark modules on a light background, whatever the theme. */}
      <rect width="100%" height="100%" className="fill-white" />
      <path d={path} className="fill-black" />
    </svg>
  )
}
