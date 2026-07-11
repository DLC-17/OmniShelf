const USD = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' })

/** Formats a card market price as US dollars, e.g. 12.34 → "$12.34". */
export function formatUsd(price: number): string {
  return USD.format(price)
}
