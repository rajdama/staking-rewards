import { BASE_MULTIPLIER, SECONDS_PER_YEAR } from "./config";

/**
 * APR (%) for a new stake of `amount` at `multiplierBps`, assuming 1 STK is worth 1 RWD
 * and the current reward rate continues for a year. Adding the stake dilutes the pool,
 * so it's included in the denominator.
 */
export function estimateApr(
  rewardRate: bigint,
  totalEffective: bigint,
  amount: bigint,
  multiplierBps: number,
  periodActive: boolean,
): number {
  if (!periodActive || rewardRate === 0n || amount === 0n) return 0;
  const effective = (amount * BigInt(multiplierBps)) / BigInt(BASE_MULTIPLIER);
  const yearlyToUser = (rewardRate * SECONDS_PER_YEAR * effective) / (totalEffective + effective);
  // Percent with 2 decimals of precision.
  return Number((yearlyToUser * 10_000n) / amount) / 100;
}

/**
 * Rewards earned since `fetchedAtMs`, computed locally so the number ticks up every second
 * between reads. Mirrors the contract's math and stops at the end of the period.
 */
export function liveEarned(
  earned: bigint,
  effective: bigint,
  totalEffective: bigint,
  rewardRate: bigint,
  finishAt: number,
  fetchedAtMs: number,
  nowSeconds: number,
): bigint {
  if (totalEffective === 0n || effective === 0n || fetchedAtMs === 0) return earned;
  const from = Math.floor(fetchedAtMs / 1000);
  const to = Math.min(nowSeconds, finishAt);
  if (to <= from) return earned;
  return earned + (effective * rewardRate * BigInt(to - from)) / totalEffective;
}
