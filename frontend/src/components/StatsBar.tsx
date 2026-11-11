"use client";

import { parseEther } from "viem";
import type { ProtocolData } from "@/hooks/useProtocol";
import { useNow } from "@/hooks/useNow";
import { estimateApr } from "@/lib/apr";
import { fmt, fmtDuration, fmtPercent } from "@/lib/format";

export function StatsBar({ protocol }: { protocol: ProtocolData }) {
  const now = useNow();
  const { totalStaked, totalEffective, rewardRate, finishAt } = protocol;
  const active = finishAt !== undefined && now < finishAt;

  const perDay = rewardRate !== undefined && active ? rewardRate * 86_400n : 0n;
  const baseApr =
    rewardRate !== undefined && totalEffective !== undefined
      ? estimateApr(rewardRate, totalEffective, parseEther("1000"), 10_000, active)
      : undefined;

  return (
    <div className="stats">
      <Stat label="Total staked" value={`${fmt(totalStaked, 0)} STK`} sub={`${fmt(totalEffective, 0)} boosted`} />
      <Stat label="Rewards per day" value={`${fmt(perDay, 1)} RWD`} sub="shared by all stakers" />
      <Stat
        label="APR (no lock)"
        value={baseApr === undefined ? "–" : active ? fmtPercent(baseApr) : "Paused"}
        sub="up to 3x with a 1-year lock"
      />
      <Stat
        label="Period ends in"
        value={finishAt === undefined ? "–" : active ? fmtDuration(finishAt - now) : "Ended"}
        sub={active ? "renews automatically" : "waiting for the next period"}
      />
    </div>
  );
}

function Stat({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div className="stat">
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
      <div className="stat-sub">{sub}</div>
    </div>
  );
}
