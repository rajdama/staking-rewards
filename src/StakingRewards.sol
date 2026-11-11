// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title StakingRewards
/// @notice Stake `stakingToken`, earn `rewardsToken` over a fixed reward period.
///
/// HOW THE MATH WORKS (the core idea of this project):
///
/// Rewards stream out at a fixed `rewardRate` (tokens per second) and are shared
/// by everyone staked, in proportion to their stake. Looping over every user each
/// second would be impossibly expensive, so instead we track ONE global number:
///
///     rewardPerToken = total rewards one staked token has earned since the start
///
/// Every time anything changes (stake / withdraw / claim), we bump it by:
///
///     rewardRate * secondsPassed / totalStaked
///
/// Each user remembers the value of rewardPerToken the last time we settled their
/// account (`userRewardPerTokenPaid`). Their new earnings are simply:
///
///     balance * (rewardPerToken - userRewardPerTokenPaid)
///
/// That makes every operation O(1), no matter how many stakers there are.
/// Values are scaled by 1e18 so the division doesn't round small numbers to zero.
contract StakingRewards is Ownable, ReentrancyGuard {
    using SafeERC20 for IERC20;

    // ------------------------------------------------------------------
    // Errors
    // ------------------------------------------------------------------
    error ZeroAmount();
    error InsufficientBalance();
    error RewardPeriodActive();
    error RewardRateZero();
    error RewardTooHigh();

    // ------------------------------------------------------------------
    // Events
    // ------------------------------------------------------------------
    event Staked(address indexed user, uint256 amount);
    event Withdrawn(address indexed user, uint256 amount);
    event RewardPaid(address indexed user, uint256 reward);
    event RewardAdded(uint256 reward, uint256 finishAt);
    event RewardsDurationUpdated(uint256 newDuration);

    // ------------------------------------------------------------------
    // Storage
    // ------------------------------------------------------------------
    IERC20 public immutable stakingToken;
    IERC20 public immutable rewardsToken;

    uint256 public duration; // length of a reward period in seconds
    uint256 public finishAt; // timestamp when the current period ends
    uint256 public updatedAt; // last time rewardPerTokenStored was updated
    uint256 public rewardRate; // reward tokens paid out per second
    uint256 public rewardPerTokenStored; // accumulated reward per staked token, scaled by 1e18

    mapping(address => uint256) public userRewardPerTokenPaid;
    mapping(address => uint256) public rewards; // settled but unclaimed rewards

    uint256 public totalSupply; // total tokens staked
    mapping(address => uint256) public balanceOf; // tokens staked per user

    constructor(address _stakingToken, address _rewardsToken, uint256 _duration) Ownable(msg.sender) {
        stakingToken = IERC20(_stakingToken);
        rewardsToken = IERC20(_rewardsToken);
        duration = _duration;
    }

    // ------------------------------------------------------------------
    // The key modifier: settle global + user accounting BEFORE any change
    // ------------------------------------------------------------------
    modifier updateReward(address account) {
        rewardPerTokenStored = rewardPerToken();
        updatedAt = lastTimeRewardApplicable();

        if (account != address(0)) {
            rewards[account] = earned(account);
            userRewardPerTokenPaid[account] = rewardPerTokenStored;
        }
        _;
    }

    // ------------------------------------------------------------------
    // Views
    // ------------------------------------------------------------------

    /// @notice Rewards stop accruing once the period has finished.
    function lastTimeRewardApplicable() public view returns (uint256) {
        return block.timestamp < finishAt ? block.timestamp : finishAt;
    }

    function rewardPerToken() public view returns (uint256) {
        if (totalSupply == 0) {
            // Nobody is staked, so nothing accrues.
            return rewardPerTokenStored;
        }
        return rewardPerTokenStored + (rewardRate * (lastTimeRewardApplicable() - updatedAt) * 1e18) / totalSupply;
    }

    function earned(address account) public view returns (uint256) {
        return (balanceOf[account] * (rewardPerToken() - userRewardPerTokenPaid[account])) / 1e18 + rewards[account];
    }

    // ------------------------------------------------------------------
    // User actions
    // ------------------------------------------------------------------

    function stake(uint256 amount) external nonReentrant updateReward(msg.sender) {
        if (amount == 0) revert ZeroAmount();

        // Effects before interactions (checks-effects-interactions pattern)
        totalSupply += amount;
        balanceOf[msg.sender] += amount;

        stakingToken.safeTransferFrom(msg.sender, address(this), amount);
        emit Staked(msg.sender, amount);
    }

    function withdraw(uint256 amount) public nonReentrant updateReward(msg.sender) {
        if (amount == 0) revert ZeroAmount();
        if (amount > balanceOf[msg.sender]) revert InsufficientBalance();

        totalSupply -= amount;
        balanceOf[msg.sender] -= amount;

        stakingToken.safeTransfer(msg.sender, amount);
        emit Withdrawn(msg.sender, amount);
    }

    function getReward() public nonReentrant updateReward(msg.sender) {
        uint256 reward = rewards[msg.sender];
        if (reward > 0) {
            rewards[msg.sender] = 0;
            rewardsToken.safeTransfer(msg.sender, reward);
            emit RewardPaid(msg.sender, reward);
        }
    }

    /// @notice Withdraw everything and claim rewards in one transaction.
    function exit() external {
        withdraw(balanceOf[msg.sender]);
        getReward();
    }

    // ------------------------------------------------------------------
    // Owner actions
    // ------------------------------------------------------------------

    function setRewardsDuration(uint256 _duration) external onlyOwner {
        // Changing the duration mid-period would break the rate math.
        if (block.timestamp < finishAt) revert RewardPeriodActive();
        duration = _duration;
        emit RewardsDurationUpdated(_duration);
    }

    /// @notice Pulls `amount` reward tokens from the owner and starts (or extends) a reward period.
    /// Any rewards left over from an active period are rolled into the new one.
    function notifyRewardAmount(uint256 amount) external onlyOwner updateReward(address(0)) {
        rewardsToken.safeTransferFrom(msg.sender, address(this), amount);

        if (block.timestamp >= finishAt) {
            rewardRate = amount / duration;
        } else {
            uint256 remaining = (finishAt - block.timestamp) * rewardRate;
            rewardRate = (amount + remaining) / duration;
        }

        if (rewardRate == 0) revert RewardRateZero();

        // Safety check: the contract must actually hold enough to pay out the full period.
        // If staking and rewards are the same token, staked funds must not count as rewards.
        uint256 available = rewardsToken.balanceOf(address(this));
        if (address(rewardsToken) == address(stakingToken)) {
            available -= totalSupply;
        }
        if (rewardRate * duration > available) revert RewardTooHigh();

        finishAt = block.timestamp + duration;
        updatedAt = block.timestamp;
        emit RewardAdded(amount, finishAt);
    }
}
