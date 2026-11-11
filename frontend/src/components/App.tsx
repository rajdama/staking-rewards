"use client";

import { ConnectButton } from "@rainbow-me/rainbowkit";
import { chain, explorerUrl, isConfigured, stakingAddress } from "@/lib/config";
import { useProtocol, useUserData } from "@/hooks/useProtocol";
import { StatsBar } from "./StatsBar";
import { StakePanel } from "./StakePanel";
import { PositionPanel } from "./PositionPanel";
import { IndexerPanels } from "./IndexerPanels";
import { TxToast } from "./TxToast";

export default function App() {
  const protocol = useProtocol();
  const user = useUserData(protocol.stakeToken, protocol.rewardToken);

  return (
    <div className="container">
      <header className="header">
        <div className="brand">
          <div className="logo" aria-hidden />
          Staking Rewards
          <span className="badge">{chain.id === 31337 ? "Local chain" : `${chain.name} testnet`}</span>
        </div>
        <ConnectButton chainStatus="icon" showBalance={false} />
      </header>

      <section className="hero">
        <h1>Stake tokens, earn every second.</h1>
        <p>
          Lock up test STK tokens and earn RWD rewards, streamed every second and shared by everyone staked. Lock for
          longer to earn up to 3x more. Free test tokens, no real money.
        </p>
      </section>

      {!isConfigured ? (
        <div className="notice" style={{ marginTop: 24 }}>
          <strong>Contract not configured.</strong> Set <code>NEXT_PUBLIC_STAKING_ADDRESS</code> to the deployed
          StakingRewards address (see <code>.env.example</code>).
        </div>
      ) : protocol.error ? (
        <div className="notice" style={{ marginTop: 24 }}>
          <strong>Can't reach the contract.</strong> Check that it's deployed on {chain.name} and the RPC URL works.
        </div>
      ) : (
        <>
          <StatsBar protocol={protocol} />
          <div className="grid">
            <StakePanel protocol={protocol} user={user} />
            <PositionPanel protocol={protocol} user={user} />
          </div>
          <IndexerPanels />
        </>
      )}

      <footer className="footer">
        Open source on{" "}
        <a href="https://github.com/rajdama/staking-rewards" target="_blank" rel="noreferrer">
          GitHub
        </a>
        {stakingAddress && explorerUrl && (
          <>
            {" · "}
            <a href={`${explorerUrl}/address/${stakingAddress}#code`} target="_blank" rel="noreferrer">
              Verified contract
            </a>
          </>
        )}
        {" · Testnet demo, not audited by a third party."}
      </footer>

      <TxToast />
    </div>
  );
}
