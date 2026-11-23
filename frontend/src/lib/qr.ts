/** Light modules kept around a QR code so that it can be scanned. */
const quietZone = 4

/** SVG path drawing the dark modules ("1") of a QR code. */
export function qrPath(rows: string[]): string {
  let path = ''
  rows.forEach((row, y) => {
    for (let x = 0; x < row.length; x++) {
      if (row[x] === '1') path += `M${x + quietZone} ${y + quietZone}h1v1h-1z`
    }
  })
  return path
}

export function qrViewBox(size: number): string {
  const side = size + 2 * quietZone
  return `0 0 ${side} ${side}`
}
