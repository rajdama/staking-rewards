"use client";

import { useAccount } from "wagmi";
import { stakingAbi } from "@/lib/abi";
import { BASE_MULTIPLIER, stakingAddress } from "@/lib/config";
import { liveEarned } from "@/lib/apr";
import { fmt, fmtDate, fmtDuration, fmtMultiplier } from "@/lib/format";
import type { ProtocolData, UserData } from "@/hooks/useProtocol";
import { useNow } from "@/hooks/useNow";
import { useTx } from "@/hooks/useTx";

export function PositionPanel({ protocol, user }: { protocol: ProtocolData; user: UserData }) {
  const { isConnected } = useAccount();
  const { send, busy } = useTx();
  const now = useNow();

  if (!isConnected) {
    return (
      <div className="card">
        <h2>Your rewards</h2>
        <p className="empty">Connect a wallet to see your positions and rewards.</p>
      </div>
    );
  }

  const earned = liveEarned(
    user.earned ?? 0n,
    user.effective ?? 0n,
    protocol.totalEffective ?? 0n,
    protocol.rewardRate ?? 0n,
    protocol.finishAt ?? 0,
    user.fetchedAt,
    now,
  );

  const open = user.positions.filter((p) => p.amount > 0n);
  const unlocked = open.filter((p) => p.unlockTime <= now);
  const share =
    protocol.totalEffective && protocol.totalEffective > 0n && user.effective
      ? Number((user.effective * 10_000n) / protocol.totalEffective) / 100
      : 0;

  async function onClaim() {
    if (!stakingAddress) return;
    await send("Claim rewards", { address: stakingAddress, abi: stakingAbi, functionName: "getReward" });
  }

  async function onExit() {
    if (!stakingAddress) return;
    await send("Withdraw unlocked + claim", { address: stakingAddress, abi: stakingAbi, functionName: "exit" });
  }

  async function onWithdraw(id: number, amount: bigint) {
    if (!stakingAddress) return;
    await send(`Withdraw ${fmt(amount)} STK`, {
      address: stakingAddress,
      abi: stakingAbi,
      functionName: "withdraw",
      args: [BigInt(id)],
    });
  }

  return (
    <div className="stack">
      <div className="card">
        <h2>Your rewards</h2>
        <div className="big-number">{fmt(earned, 6)} RWD</div>
        <p className="hint" style={{ marginTop: 4 }}>
          Updates every second while you're staked.
        </p>

        <div style={{ marginTop: 16 }}>
          <div className="kv">
            <span>Staked</span>
            <span>{fmt(user.staked)} STK</span>
          </div>
          <div className="kv">
            <span>Boosted balance</span>
            <span>{fmt(user.effective)}</span>
          </div>
          <div className="kv">
            <span>Your share of the pool</span>
            <span>{share.toLocaleString("en-US", { maximumFractionDigits: 2 })}%</span>
          </div>
          <div className="kv">
            <span>RWD in wallet</span>
            <span>{fmt(user.rewardBalance)} RWD</span>
          </div>
        </div>

        <div className="btn-row" style={{ marginTop: 16 }}>
          <button className="btn" onClick={onClaim} disabled={busy || earned === 0n}>
            Claim
          </button>
          <button className="btn secondary" onClick={onExit} disabled={busy || (unlocked.length === 0 && earned === 0n)}>
            Exit
          </button>
        </div>
        <p className="hint">Exit withdraws every unlocked position and claims your rewards in one transaction.</p>
      </div>

      <div className="card">
        <h2>Your positions</h2>
        {open.length === 0 ? (
          <p className="empty">No open positions yet.</p>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Amount</th>
                  <th>Boost</th>
                  <th>Unlocks</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {open.map((p) => {
                  const isUnlocked = p.unlockTime <= now;
                  const boostExpired = isUnlocked && p.multiplierBps > BASE_MULTIPLIER;
                  return (
                    <tr key={p.id}>
                      <td>{fmt(p.amount)} STK</td>
                      <td>
                        {fmtMultiplier(p.multiplierBps)}
                        {boostExpired && (
                          <span className="pill warn" style={{ marginLeft: 6 }} title="Anyone can reset an expired boost to 1x">
                            expired
                          </span>
                        )}
                      </td>
                      <td>
                        {isUnlocked ? (
                          <span className="pill good">Unlocked</span>
                        ) : (
                          <span title={fmtDate(p.unlockTime)}>in {fmtDuration(p.unlockTime - now)}</span>
                        )}
                      </td>
                      <td>
                        <button
                          className="btn secondary small"
                          disabled={busy || !isUnlocked}
                          onClick={() => onWithdraw(p.id, p.amount)}
                        >
                          Withdraw
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
