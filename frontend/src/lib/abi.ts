import { parseAbi } from "viem";

export const stakingAbi = parseAbi([
  "function stakingToken() view returns (address)",
  "function rewardsToken() view returns (address)",
  "function totalStaked() view returns (uint256)",
  "function totalEffective() view returns (uint256)",
  "function rewardRate() view returns (uint256)",
  "function finishAt() view returns (uint256)",
  "function duration() view returns (uint256)",
  "function stakedOf(address user) view returns (uint256)",
  "function effectiveBalanceOf(address user) view returns (uint256)",
  "function earned(address user) view returns (uint256)",
  "function getPositions(address user) view returns ((uint128 amount, uint64 unlockTime, uint16 multiplierBps)[])",
  "function stake(uint256 amount, uint8 lockId) returns (uint256)",
  "function withdraw(uint256 positionId)",
  "function getReward()",
  "function exit()",
  "function kick(address user, uint256 positionId)",
  "error ZeroAmount()",
  "error InvalidLockOption(uint8 lockId)",
  "error InvalidPosition(uint256 positionId)",
  "error StillLocked(uint256 unlockTime)",
  "error NotKickable()",
  "error ERC20InsufficientBalance(address sender, uint256 balance, uint256 needed)",
  "error ERC20InsufficientAllowance(address spender, uint256 allowance, uint256 needed)",
]);

export const tokenAbi = parseAbi([
  "function symbol() view returns (string)",
  "function balanceOf(address owner) view returns (uint256)",
  "function allowance(address owner, address spender) view returns (uint256)",
  "function approve(address spender, uint256 amount) returns (bool)",
  "function faucet()",
  "function faucetAmount() view returns (uint256)",
  "function lastFaucetAt(address user) view returns (uint256)",
  "error FaucetCooldown(uint256 availableAt)",
]);
