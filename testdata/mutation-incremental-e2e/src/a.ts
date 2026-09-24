export function grade(score: number, pass: number, top: number): string {
  if (score < pass) return 'fail';
  if (score > top) return 'over';
  return 'ok';
}
