"use client";

import { useState } from "react";
import { formatEther, maxUint256, parseEther } from "viem";
import { useAccount } from "wagmi";
import { ConnectButton } from "@rainbow-me/rainbowkit";
import { stakingAbi, tokenAbi } from "@/lib/abi";
import { LOCK_OPTIONS, stakingAddress } from "@/lib/config";
import { estimateApr } from "@/lib/apr";
import { fmt, fmtDuration, fmtMultiplier, fmtPercent } from "@/lib/format";
import type { ProtocolData, UserData } from "@/hooks/useProtocol";
import { useNow } from "@/hooks/useNow";
import { useTx } from "@/hooks/useTx";

const FAUCET_COOLDOWN = 86_400;

function parseAmount(input: string): bigint {
  try {
    return input.trim() === "" ? 0n : parseEther(input.trim());
  } catch {
    return 0n;
  }
}

export function StakePanel({ protocol, user }: { protocol: ProtocolData; user: UserData }) {
  const { isConnected } = useAccount();
  const { send, busy } = useTx();
  const now = useNow();
  const [input, setInput] = useState("");
  const [lockId, setLockId] = useState(0);

  const amount = parseAmount(input);
  const balance = user.stakeBalance ?? 0n;
  const needsApproval = amount > 0n && (user.allowance ?? 0n) < amount;
  const tooMuch = amount > balance;
  const active = protocol.finishAt !== undefined && now < protocol.finishAt;

  const faucetReadyAt = user.lastFaucetAt ? user.lastFaucetAt + FAUCET_COOLDOWN : 0;
  const faucetReady = now >= faucetReadyAt;

  const aprAmount = amount > 0n ? amount : parseEther("1000");
  const aprFor = (multiplierBps: number) =>
    protocol.rewardRate !== undefined && protocol.totalEffective !== undefined
      ? estimateApr(protocol.rewardRate, protocol.totalEffective, aprAmount, multiplierBps, active)
      : 0;

  async function onFaucet() {
    if (!protocol.stakeToken) return;
    await send("Get test tokens", { address: protocol.stakeToken, abi: tokenAbi, functionName: "faucet" });
  }

  async function onApprove() {
    if (!protocol.stakeToken || !stakingAddress) return;
    await send("Approve STK", {
      address: protocol.stakeToken,
      abi: tokenAbi,
      functionName: "approve",
      args: [stakingAddress, maxUint256],
    });
  }

  async function onStake() {
    if (!stakingAddress) return;
    const ok = await send(`Stake ${fmt(amount)} STK`, {
      address: stakingAddress,
      abi: stakingAbi,
      functionName: "stake",
      args: [amount, lockId],
    });
    if (ok) setInput("");
  }

  const lock = LOCK_OPTIONS[lockId];

  return (
    <div className="card">
      <div className="card-head">
        <h2>Stake</h2>
        {isConnected && (
          <button className="link-btn" onClick={onFaucet} disabled={busy || !faucetReady}>
            {faucetReady ? `Get ${fmt(user.faucetAmount, 0)} free STK` : `Faucet again in ${fmtDuration(faucetReadyAt - now)}`}
          </button>
        )}
      </div>

      <div className="field-label">
        <span>Amount</span>
        <span>
          Balance: {fmt(balance)} STK{" "}
          <button className="link-btn" onClick={() => setInput(balance > 0n ? formatEther(balance) : "")}>
            Max
          </button>
        </span>
      </div>
      <div className="input-row">
        <input
          inputMode="decimal"
          placeholder="0.0"
          value={input}
          onChange={(e) => setInput(e.target.value.replace(/[^0-9.]/g, ""))}
          aria-label="Amount to stake"
        />
      </div>

      <div className="locks" role="radiogroup" aria-label="Lock period">
        {LOCK_OPTIONS.map((o) => (
          <button
            key={o.id}
            role="radio"
            aria-checked={lockId === o.id}
            className={`lock ${lockId === o.id ? "selected" : ""}`}
            onClick={() => setLockId(o.id)}
          >
            <div className="lock-mult">{fmtMultiplier(o.multiplierBps)}</div>
            <div className="lock-label">{o.label}</div>
            <div className="lock-apr">{active ? `~${fmtPercent(aprFor(o.multiplierBps))} APR` : "–"}</div>
          </button>
        ))}
      </div>

      {!isConnected ? (
        <ConnectButton.Custom>
          {({ openConnectModal }) => (
            <button className="btn" onClick={openConnectModal}>
              Connect wallet to stake
            </button>
          )}
        </ConnectButton.Custom>
      ) : needsApproval ? (
        <button className="btn" onClick={onApprove} disabled={busy || tooMuch}>
          {tooMuch ? "Not enough STK" : "Approve STK"}
        </button>
      ) : (
        <button className="btn" onClick={onStake} disabled={busy || amount === 0n || tooMuch}>
          {tooMuch ? "Not enough STK" : amount === 0n ? "Enter an amount" : `Stake with ${fmtMultiplier(lock.multiplierBps)} boost`}
        </button>
      )}

      <p className="hint">
        {lock.days === 0
          ? "No lock: withdraw any time."
          : `Locked for ${lock.days} days, then withdraw any time. Longer locks earn a bigger share of rewards.`}{" "}
        APR assumes 1 STK = 1 RWD.
      </p>
    </div>
  );
}
