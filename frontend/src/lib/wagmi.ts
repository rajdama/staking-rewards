import { getDefaultConfig } from "@rainbow-me/rainbowkit";
import { http } from "wagmi";
import { chain, rpcUrl, walletConnectProjectId } from "./config";

export const wagmiConfig = getDefaultConfig({
  appName: "Staking Rewards",
  projectId: walletConnectProjectId,
  chains: [chain] as const,
  transports: { [chain.id]: http(rpcUrl) },
  ssr: true,
});
