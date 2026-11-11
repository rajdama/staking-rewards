"use client";

import { useQuery } from "@tanstack/react-query";
import { useAccount } from "wagmi";
import { explorerUrl, indexerUrl } from "@/lib/config";
import { fmt, fmtMultiplier, shortAddress } from "@/lib/format";

type ActivityItem = {
  type: "stake" | "withdraw" | "claim" | "kick";
  user: string;
  positionId?: number;
  amount?: string;
  multiplierBps?: number;
  txHash: string;
  blockNumber: number;
  timestamp: number;
};

type LeaderboardItem = { user: string; staked: string; effective: string; claimed: string };
type HistoryPoint = { timestamp: number; totalStaked: string };
type Stats = { stakers: number; openPositions: number; totalClaimed: string; lastIndexedBlock: number };

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${indexerUrl}${path}`);
  if (!res.ok) throw new Error(`Indexer returned ${res.status}`);
  return res.json() as Promise<T>;
}

function useIndexer<T>(key: string, path: string) {
  return useQuery({
    queryKey: ["indexer", key],
    queryFn: () => get<T>(path),
    enabled: indexerUrl !== "",
    refetchInterval: 15_000,
  });
}

const LABELS: Record<ActivityItem["type"], string> = {
  stake: "Staked",
  withdraw: "Withdrew",
  claim: "Claimed",
  kick: "Boost reset",
};

function timeAgo(ts: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000) - ts);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

export function IndexerPanels() {
  const { address } = useAccount();
  const stats = useIndexer<Stats>("stats", "/api/stats");
  const activity = useIndexer<{ items: ActivityItem[] }>("activity", "/api/activity?limit=12");
  const leaders = useIndexer<{ items: LeaderboardItem[] }>("leaderboard", "/api/leaderboard?limit=8");
  const history = useIndexer<{ items: HistoryPoint[] }>("history", "/api/history?days=30");

  if (indexerUrl === "") return null;
  if (stats.isError) {
    return <div className="notice">The activity indexer isn't reachable right now. On-chain data above is still live.</div>;
  }

  return (
    <div className="grid" style={{ marginTop: 16 }}>
      <div className="stack">
        <div className="card">
          <div className="card-head">
            <h2>Total staked, last 30 days</h2>
            {stats.data && <span className="badge">{stats.data.stakers} stakers</span>}
          </div>
          <StakedChart points={history.data?.items ?? []} />
        </div>

        <div className="card">
          <h2>Leaderboard</h2>
          {(leaders.data?.items.length ?? 0) === 0 ? (
            <p className="empty">No stakers yet.</p>
          ) : (
            <table className="table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>Staker</th>
                  <th>Boosted</th>
                  <th>Claimed</th>
                </tr>
              </thead>
              <tbody>
                {leaders.data!.items.map((l, i) => (
                  <tr key={l.user}>
                    <td>{i + 1}</td>
                    <td className="mono">
                      {shortAddress(l.user)}
                      {address && l.user.toLowerCase() === address.toLowerCase() && (
                        <span className="pill good" style={{ marginLeft: 6 }}>
                          you
                        </span>
                      )}
                    </td>
                    <td>{fmt(BigInt(l.effective), 0)}</td>
                    <td>{fmt(BigInt(l.claimed), 1)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="card">
        <h2>Recent activity</h2>
        {(activity.data?.items.length ?? 0) === 0 ? (
          <p className="empty">No activity yet.</p>
        ) : (
          <table className="table">
            <tbody>
              {activity.data!.items.map((a) => (
                <tr key={`${a.txHash}-${a.type}-${a.positionId ?? ""}`}>
                  <td>
                    <span className="pill">{LABELS[a.type]}</span>
                  </td>
                  <td>
                    {a.amount ? `${fmt(BigInt(a.amount))} ${a.type === "claim" ? "RWD" : "STK"}` : ""}
                    {a.type === "stake" && a.multiplierBps ? ` · ${fmtMultiplier(a.multiplierBps)}` : ""}
                  </td>
                  <td className="mono">{shortAddress(a.user)}</td>
                  <td>
                    <a href={`${explorerUrl}/tx/${a.txHash}`} target="_blank" rel="noreferrer">
                      {timeAgo(a.timestamp)}
                    </a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

/** A small area chart drawn as SVG, so no chart library is needed. */
function StakedChart({ points }: { points: HistoryPoint[] }) {
  if (points.length < 2) return <p className="empty">Not enough history yet.</p>;

  const W = 500;
  const H = 120;
  const values = points.map((p) => Number(BigInt(p.totalStaked) / 10n ** 15n) / 1000);
  const max = Math.max(...values, 1);
  const x = (i: number) => (i / (points.length - 1)) * W;
  const y = (v: number) => H - 8 - (v / max) * (H - 16);
  const line = values.map((v, i) => `${x(i)},${y(v)}`).join(" ");
  const last = values[values.length - 1];

  return (
    <>
      <svg className="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label="Total staked over time">
        <defs>
          <linearGradient id="fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="#7c5cff" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#7c5cff" stopOpacity="0" />
          </linearGradient>
        </defs>
        <polygon points={`0,${H} ${line} ${W},${H}`} fill="url(#fill)" />
        <polyline points={line} fill="none" stroke="#a28bff" strokeWidth="2" vectorEffect="non-scaling-stroke" />
      </svg>
      <p className="hint">Now: {last.toLocaleString("en-US", { maximumFractionDigits: 0 })} STK</p>
    </>
  );
}
