import { isAddress, type Address } from "viem";
import { foundry, sepolia, type Chain } from "wagmi/chains";

const chainId = Number(process.env.NEXT_PUBLIC_CHAIN_ID || sepolia.id);
const stakingEnv = process.env.NEXT_PUBLIC_STAKING_ADDRESS || "";

export const chain: Chain = chainId === foundry.id ? foundry : sepolia;
export const rpcUrl = process.env.NEXT_PUBLIC_RPC_URL || undefined;
export const walletConnectProjectId = process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID || "staking-rewards-demo";
export const indexerUrl = (process.env.NEXT_PUBLIC_INDEXER_URL || "").replace(/\/$/, "");

export const stakingAddress: Address | undefined = isAddress(stakingEnv) ? stakingEnv : undefined;
export const isConfigured = stakingAddress !== undefined;

export const explorerUrl = chain.blockExplorers?.default.url ?? "";

/** Mirrors StakingRewards.lockOption(). */
export const LOCK_OPTIONS = [
  { id: 0, label: "No lock", days: 0, multiplierBps: 10_000 },
  { id: 1, label: "30 days", days: 30, multiplierBps: 15_000 },
  { id: 2, label: "90 days", days: 90, multiplierBps: 20_000 },
  { id: 3, label: "1 year", days: 365, multiplierBps: 30_000 },
] as const;

export const BASE_MULTIPLIER = 10_000;
export const SECONDS_PER_YEAR = 365n * 24n * 60n * 60n;
