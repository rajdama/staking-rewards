# Self-Audit: StakingRewards

An internal security review of `contracts/src/StakingRewards.sol` and `contracts/src/FaucetToken.sol`.
This is a self-review for a learning and portfolio project, **not** a third-party audit.
Don't use these contracts with real funds.

## Summary

| Severity | Found | Fixed | Mitigated | Acknowledged |
|---|---|---|---|---|
| High | 0 | – | – | – |
| Medium | 2 | 1 | 1 | – |
| Low | 3 | 2 | 1 | – |
| Informational | 5 | – | – | 5 |

All findings below were either fixed in code, mitigated with a design change, or are accepted
trade-offs documented here.

## Scope and method

| Item | Detail |
|---|---|
| Contracts | `StakingRewards.sol`, `FaucetToken.sol` (OpenZeppelin v5.1.0 for ERC20, Ownable, ReentrancyGuard, SafeERC20, SafeCast) |
| Compiler | Solidity 0.8.24, optimizer 200 runs |
| Unit tests | 47 tests covering every function, every revert and every branch |
| Fuzz tests | 2 properties × 1,000 random runs each |
| Invariant tests | 4 properties, 256 runs × 100 random calls (25,600 calls per CI run), `fail_on_revert` on |
| Coverage | 100% lines, statements, branches and functions |
| Static analysis | Slither (102 detectors) on every push; CI fails on any medium or high finding |
| End-to-end | The real deploy and renew scripts run against a local Anvil chain on every push |

### Invariants checked after every random action sequence

1. **Staked tokens are backed:** `totalStaked` equals the sum of user stakes and the contract's staking-token balance.
2. **Effective balances add up:** `totalEffective` equals the sum of users' boosted balances, and no boosted balance is below its raw stake.
3. **Solvency:** the reward-token balance always covers all unclaimed rewards plus undistributed rewards.
4. **Conservation:** reward tokens funded = claimed + recovered + still held by the contract.

## Findings

### M-1: Rewards were locked forever when nobody was staked (Fixed)

**Where:** reward accounting (original Synthetix-style design).
**Issue:** when `totalSupply == 0`, `rewardPerToken` doesn't grow, but time still passes and the reward
schedule still "spends" tokens. Those rewards were never assigned to anyone and stayed in the contract
with no way out. With a 7-day period and a quiet first day, about 14% of each period's rewards were lost.
**Fix:** `_updateReward` now adds `rewardRate × elapsed` to `undistributedRewards` whenever nobody is
staked, and the owner can send it out with `recoverUndistributed`. Covered by
`test_UndistributedRewardsAreTrackedAndRecoverable`, `test_UndistributedCountsGapsMidPeriod` and the
conservation invariant.

### M-2: Expired locks kept their boost (Mitigated)

**Where:** lock boosts.
**Issue:** a position's multiplier applies until something changes it. Without intervention, a 1-year
3x lock would keep earning 3x after it unlocks, taking rewards from everyone else.
**Mitigation:** `kick(user, positionId)` lets **anyone** reset an expired boost to 1x, and the Go indexer
ships with an optional keeper that does this automatically. The boost can be over-applied only between
expiry and the next kick (minutes with the keeper running). Covered by `test_KickResetsExpiredBoost`
and `testFuzz_KickNeverIncreasesEffective`.

### L-1: Lock boost could be gamed by topping up an old lock (Fixed by design)

**Issue:** an early design used one lock per user. A user could lock 1 wei for a year, wait 364 days,
then add a large amount and get a 3x boost for a 1-day lock.
**Fix:** every stake opens a separate **position** with its own lock and multiplier, so a boost always
matches the lock of the tokens it applies to.

### L-2: Solvency check compared a rounded-down rate (Fixed)

**Where:** `notifyRewardAmount`. Reported by Slither as `divide-before-multiply`.
**Issue:** the check compared `rewardRate × duration` (rounded down) against the balance, slightly
understating what the period promises.
**Fix:** the check now compares the exact total (`amount + remaining`) before dividing, which is
stricter. `RewardRateZero` uses `total < duration` instead of a strict `rewardRate == 0` equality.

### L-3: Fee-on-transfer tokens (Mitigated for rewards, unsupported for staking)

**Issue:** tokens that take a fee on transfer deliver less than the amount sent.
**Mitigation:** for the reward token, the solvency check reverts with `RewardTooHigh` because the
contract receives less than the promised total (`test_RevertWhen_FeeOnTransferRewardTooHigh`).
For the staking token, fee-on-transfer and rebasing tokens are **not supported**: a deployer must use a
standard ERC20.

### I-1: Timestamp dependence (Acknowledged)

Slither's `timestamp` detector flags comparisons with `block.timestamp`. Validators can only shift
timestamps by a few seconds, which is negligible against reward periods measured in days and locks
measured in weeks. This is the same trade-off every time-based staking contract makes.

### I-2: Remaining-reward calculation flagged as divide-before-multiply (Acknowledged, false positive)

`(finishAt - block.timestamp) × rewardRate` multiplies a stored rate that came from a division. This is
intended: `rewardRate` is the exact rate still being paid, so the product is exactly what the current
schedule has left. Suppressed inline with an explanation.

### I-3: `exit()` gas grows with a user's positions (Acknowledged)

`exit()` loops over the caller's own positions. Storage writes for totals are batched outside the loop,
and users with many positions can always call `withdraw(positionId)` per position instead. Only the
caller is affected.

### I-4: Rounding dust (Acknowledged)

Reward math rounds down, so tiny amounts (wei) stay in the contract. The solvency and conservation
invariants confirm rounding never favors users over the contract.

### I-5: Owner powers (Acknowledged)

The owner can start or extend reward periods, change the period length between periods, and recover
undistributed rewards. The owner **cannot** move staked tokens, change locks, or take rewards already
earned by users. A production deployment should put the owner behind a multisig and a timelock.

## Slither results

Latest run: **0 high, 0 medium**. Remaining results are informational `timestamp` notes (I-1).
The full report is produced on every push by the `contracts` workflow.
