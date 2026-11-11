#!/usr/bin/env bash
# Seeds a local Anvil chain with ~45 days of realistic staking activity, ending at the real
# current time. Used to produce screenshots and to try the full stack locally.
#
# Usage (from the repo root, with anvil started as below):
#   anvil --timestamp $(( $(date +%s) - 45*86400 )) &
#   scripts/demo/seed.sh
set -euo pipefail

RPC=${RPC:-http://127.0.0.1:8545}
DAY=86400

# Anvil's well-known test accounts. Never use these keys on a real network.
K0=0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 # owner / deployer
K1=0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d # demo user (screenshots)
K2=0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a
K3=0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6
K4=0x47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a

addr() { cast wallet address --private-key "$1"; }
A0=$(addr $K0)
A1=$(addr $K1)

send() { cast send --rpc-url "$RPC" "$@" >/dev/null; }
warp_days() {
  cast rpc --rpc-url "$RPC" evm_increaseTime $(( $1 * DAY )) >/dev/null
  cast rpc --rpc-url "$RPC" evm_mine >/dev/null
}

cd "$(dirname "$0")/../../contracts"
PRIVATE_KEY=$K0 forge script script/Deploy.s.sol --rpc-url "$RPC" --broadcast >/dev/null
STAKING=$(jq -r .staking deployments/31337.json)
STK=$(jq -r .stakeToken deployments/31337.json)
RWD=$(jq -r .rewardToken deployments/31337.json)
echo "StakingRewards: $STAKING"

stake() { # key amount lockId
  local who; who=$(addr "$1")
  send "$STK" "mint(address,uint256)" "$who" "$2ether" --private-key $K0
  send "$STK" "approve(address,uint256)" "$STAKING" "$2ether" --private-key "$1"
  send "$STAKING" "stake(uint256,uint8)" "$2ether" "$3" --private-key "$1"
}
claim() { send "$STAKING" "getReward()" --private-key "$1"; }
renew() {
  send "$RWD" "mint(address,uint256)" "$A0" 1000ether --private-key $K0
  send "$RWD" "approve(address,uint256)" "$STAKING" 1000ether --private-key $K0
  send "$STAKING" "notifyRewardAmount(uint256)" 1000ether --private-key $K0
}

# Day -45: the first reward period starts at deploy.
stake $K2 800 1   # 30-day lock, expires on day -15 (the keeper will kick it)
warp_days 1
stake $K3 1500 0
warp_days 4
stake $K1 1000 3  # demo user: 1-year lock, 3x
warp_days 2
renew             # day -38
warp_days 3
stake $K4 2000 2  # 90-day lock
warp_days 4
renew             # day -31
claim $K3
warp_days 3
stake $K1 500 0   # demo user: no lock
warp_days 4
renew             # day -24
warp_days 4
send "$STAKING" "withdraw(uint256)" 0 --private-key $K3   # day -20
stake $K3 600 1
warp_days 3
renew             # day -17
claim $K1
warp_days 7
renew             # day -10
warp_days 2
stake $K1 300 1   # demo user: 30-day lock
warp_days 5
renew             # day -3
claim $K4
warp_days 2

# Give the demo user some STK to stake in the screenshots, then jump to the real current time.
send "$STK" "mint(address,uint256)" "$A1" 2500ether --private-key $K0
cast rpc --rpc-url "$RPC" evm_setNextBlockTimestamp "$(date +%s)" >/dev/null
cast rpc --rpc-url "$RPC" evm_mine >/dev/null

echo "Seeded. Demo user: $A1"
