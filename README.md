# Staking Rewards

Stake an ERC-20 token and earn reward tokens over time, shared in proportion to your stake.
Built with [Foundry](https://book.getfoundry.sh/) and OpenZeppelin.

This is project 1 of a hands-on Web3 learning path: it covers reward accounting math,
the checks-effects-interactions pattern, and testing with Foundry (unit + fuzz tests).

## How it works

The owner deposits reward tokens and starts a reward period (7 days by default).
Rewards stream out at a fixed rate per second and are split among everyone staked.

Instead of looping over every user (which would cost unbounded gas), the contract tracks one
global number, `rewardPerToken`: how much one staked token has earned since the start. Each user
stores the value it had when their account was last settled, so their earnings are:

```
earned = balance × (rewardPerToken − userRewardPerTokenPaid) + unclaimed rewards
```

Every function is O(1) no matter how many stakers there are. This is the same design used by
Synthetix and many DeFi protocols that followed it.

## Getting started

```bash
# 1. Install Foundry (once)
curl -L https://foundry.paradigm.xyz | bash
foundryup

# 2. Set up the project
cd staking-rewards
git init
forge install foundry-rs/forge-std OpenZeppelin/openzeppelin-contracts

# 3. Build and test
forge build
forge test -vv
```

Useful extras:

```bash
forge test --match-test testFuzz -vvv   # run only the fuzz test
forge coverage                          # see which lines are tested
forge snapshot                          # gas report per test
```

## Deploying to Sepolia

```bash
cast wallet import dev --interactive    # store your testnet key encrypted, once
forge script script/Deploy.s.sol --rpc-url $SEPOLIA_RPC_URL --account dev --broadcast
```

Get Sepolia ETH from a faucet first. Never use a key that holds real funds.

## Project structure

```
src/StakingRewards.sol        the contract
test/StakingRewards.t.sol     unit + fuzz tests
test/mocks/MockERC20.sol      mintable token for tests
script/Deploy.s.sol           testnet deployment
```

## Exercises (do these yourself, in order)

1. **Read the math.** Before changing anything, trace `test_LateStakerOnlyEarnsFromJoinTime`
   by hand: write down `rewardPerTokenStored`, `updatedAt` and each user's
   `userRewardPerTokenPaid` after every step.
2. **Find the stuck-rewards bug.** If nobody is staked for part of the period, the rewards for
   that time are never paid to anyone and stay locked in the contract. Write a test that proves
   it, then add an owner function to recover them safely (without touching staked funds or
   rewards already owed).
3. **Write an invariant test.** Use Foundry's invariant testing with a handler contract that
   randomly stakes, withdraws and claims for several users. Invariant: the contract's reward
   token balance is always ≥ the total rewards owed to all users.
4. **Add a lock period.** Users can't withdraw for 3 days after staking. Think about what
   happens when someone stakes again during their lock.
5. **Try an attack.** What happens if the reward token charges a fee on transfer? Write a
   fee-on-transfer mock token and see which accounting breaks.
6. **Ship it.** Deploy to Sepolia, verify the contract on Etherscan, and add the addresses here.
