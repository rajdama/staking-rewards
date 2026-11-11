"use client";

import { useAccount, useReadContracts } from "wagmi";
import { zeroAddress, type Address } from "viem";
import { stakingAbi, tokenAbi } from "@/lib/abi";
import { stakingAddress } from "@/lib/config";

const REFRESH_MS = 12_000;
const staking = { address: stakingAddress ?? zeroAddress, abi: stakingAbi } as const;

export type Position = {
  id: number;
  amount: bigint;
  unlockTime: number;
  multiplierBps: number;
};

/** Protocol-wide state, readable without a wallet. */
export function useProtocol() {
  const { data, dataUpdatedAt, isLoading, error } = useReadContracts({
    allowFailure: false,
    contracts: [
      { ...staking, functionName: "totalStaked" },
      { ...staking, functionName: "totalEffective" },
      { ...staking, functionName: "rewardRate" },
      { ...staking, functionName: "finishAt" },
      { ...staking, functionName: "duration" },
      { ...staking, functionName: "stakingToken" },
      { ...staking, functionName: "rewardsToken" },
    ],
    query: { enabled: stakingAddress !== undefined, refetchInterval: REFRESH_MS },
  });

  return {
    isLoading,
    error,
    fetchedAt: dataUpdatedAt,
    totalStaked: data?.[0],
    totalEffective: data?.[1],
    rewardRate: data?.[2],
    finishAt: data ? Number(data[3]) : undefined,
    duration: data ? Number(data[4]) : undefined,
    stakeToken: data?.[5] as Address | undefined,
    rewardToken: data?.[6] as Address | undefined,
  };
}

/** The connected user's balances, positions and allowance. */
export function useUserData(stakeToken: Address | undefined, rewardToken: Address | undefined) {
  const { address } = useAccount();
  const user = address ?? zeroAddress;
  const stk = { address: stakeToken ?? zeroAddress, abi: tokenAbi } as const;
  const rwd = { address: rewardToken ?? zeroAddress, abi: tokenAbi } as const;

  const { data, dataUpdatedAt } = useReadContracts({
    allowFailure: false,
    contracts: [
      { ...staking, functionName: "stakedOf", args: [user] },
      { ...staking, functionName: "effectiveBalanceOf", args: [user] },
      { ...staking, functionName: "earned", args: [user] },
      { ...staking, functionName: "getPositions", args: [user] },
      { ...stk, functionName: "balanceOf", args: [user] },
      { ...stk, functionName: "allowance", args: [user, staking.address] },
      { ...stk, functionName: "lastFaucetAt", args: [user] },
      { ...stk, functionName: "faucetAmount" },
      { ...rwd, functionName: "balanceOf", args: [user] },
    ],
    query: {
      enabled: address !== undefined && stakingAddress !== undefined && !!stakeToken && !!rewardToken,
      refetchInterval: REFRESH_MS,
    },
  });

  const positions: Position[] = (data?.[3] ?? []).map((p, id) => ({
    id,
    amount: p.amount,
    unlockTime: Number(p.unlockTime),
    multiplierBps: Number(p.multiplierBps),
  }));

  return {
    address,
    fetchedAt: dataUpdatedAt,
    staked: data?.[0],
    effective: data?.[1],
    earned: data?.[2],
    positions,
    stakeBalance: data?.[4],
    allowance: data?.[5],
    lastFaucetAt: data ? Number(data[6]) : undefined,
    faucetAmount: data?.[7],
    rewardBalance: data?.[8],
  };
}

export type ProtocolData = ReturnType<typeof useProtocol>;
export type UserData = ReturnType<typeof useUserData>;
