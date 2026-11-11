// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {StakingRewards} from "../src/StakingRewards.sol";
import {MockERC20} from "./mocks/MockERC20.sol";

contract StakingRewardsTest is Test {
    StakingRewards internal staking;
    MockERC20 internal stakeToken;
    MockERC20 internal rewardToken;

    address internal owner = makeAddr("owner");
    address internal alice = makeAddr("alice");
    address internal bob = makeAddr("bob");

    uint256 internal constant DURATION = 7 days;
    uint256 internal constant REWARD = 1_000e18;

    function setUp() public {
        stakeToken = new MockERC20("Stake Token", "STK");
        rewardToken = new MockERC20("Reward Token", "RWD");

        vm.prank(owner);
        staking = new StakingRewards(address(stakeToken), address(rewardToken), DURATION);

        // Owner funds and starts a 7-day reward period.
        rewardToken.mint(owner, REWARD);
        vm.startPrank(owner);
        rewardToken.approve(address(staking), REWARD);
        staking.notifyRewardAmount(REWARD);
        vm.stopPrank();
    }

    // ------------------------------------------------------------------
    // Helpers
    // ------------------------------------------------------------------
    function _stake(address user, uint256 amount) internal {
        stakeToken.mint(user, amount);
        vm.startPrank(user);
        stakeToken.approve(address(staking), amount);
        staking.stake(amount);
        vm.stopPrank();
    }

    // ------------------------------------------------------------------
    // Basic behaviour
    // ------------------------------------------------------------------
    function test_StakeUpdatesBalances() public {
        _stake(alice, 100e18);

        assertEq(staking.balanceOf(alice), 100e18);
        assertEq(staking.totalSupply(), 100e18);
        assertEq(stakeToken.balanceOf(address(staking)), 100e18);
    }

    function test_EarnsHalfRewardAtHalfTime() public {
        _stake(alice, 100e18);
        vm.warp(block.timestamp + DURATION / 2);

        // Within 0.1% (integer division loses a few wei)
        assertApproxEqRel(staking.earned(alice), REWARD / 2, 1e15);
    }

    function test_TwoStakersSplitProportionally() public {
        _stake(alice, 100e18); // 25% of the pool
        _stake(bob, 300e18); // 75% of the pool
        vm.warp(block.timestamp + DURATION);

        assertApproxEqRel(staking.earned(alice), REWARD / 4, 1e15);
        assertApproxEqRel(staking.earned(bob), (REWARD * 3) / 4, 1e15);
    }

    function test_LateStakerOnlyEarnsFromJoinTime() public {
        _stake(alice, 100e18);
        vm.warp(block.timestamp + DURATION / 2);
        _stake(bob, 100e18);
        vm.warp(block.timestamp + DURATION / 2);

        // Alice: all of first half + half of second half = 75%
        // Bob:   half of second half = 25%
        assertApproxEqRel(staking.earned(alice), (REWARD * 3) / 4, 1e15);
        assertApproxEqRel(staking.earned(bob), REWARD / 4, 1e15);
    }

    function test_NoRewardsAfterPeriodEnds() public {
        _stake(alice, 100e18);
        vm.warp(block.timestamp + DURATION);
        uint256 atEnd = staking.earned(alice);

        vm.warp(block.timestamp + 30 days);
        assertEq(staking.earned(alice), atEnd);
    }

    function test_GetRewardTransfersTokens() public {
        _stake(alice, 100e18);
        vm.warp(block.timestamp + DURATION);
        uint256 expected = staking.earned(alice);

        vm.prank(alice);
        staking.getReward();

        assertEq(rewardToken.balanceOf(alice), expected);
        assertEq(staking.earned(alice), 0);
    }

    function test_ExitReturnsStakeAndRewards() public {
        _stake(alice, 100e18);
        vm.warp(block.timestamp + DURATION);
        uint256 expected = staking.earned(alice);

        vm.prank(alice);
        staking.exit();

        assertEq(stakeToken.balanceOf(alice), 100e18);
        assertEq(rewardToken.balanceOf(alice), expected);
        assertEq(staking.totalSupply(), 0);
    }

    // ------------------------------------------------------------------
    // Reverts
    // ------------------------------------------------------------------
    function test_RevertWhen_StakeZero() public {
        vm.prank(alice);
        vm.expectRevert(StakingRewards.ZeroAmount.selector);
        staking.stake(0);
    }

    function test_RevertWhen_WithdrawMoreThanStaked() public {
        _stake(alice, 100e18);

        vm.prank(alice);
        vm.expectRevert(StakingRewards.InsufficientBalance.selector);
        staking.withdraw(101e18);
    }

    function test_RevertWhen_NonOwnerNotifies() public {
        vm.prank(alice);
        vm.expectRevert(abi.encodeWithSelector(Ownable.OwnableUnauthorizedAccount.selector, alice));
        staking.notifyRewardAmount(1e18);
    }

    function test_RevertWhen_ChangingDurationMidPeriod() public {
        vm.prank(owner);
        vm.expectRevert(StakingRewards.RewardPeriodActive.selector);
        staking.setRewardsDuration(14 days);
    }

    // ------------------------------------------------------------------
    // Fuzz test: random amounts and timings must never pay out more than funded
    // ------------------------------------------------------------------
    function testFuzz_NeverPaysMoreThanFunded(uint256 aliceAmt, uint256 bobAmt, uint256 bobJoinsAfter) public {
        aliceAmt = bound(aliceAmt, 1, 1e30);
        bobAmt = bound(bobAmt, 1, 1e30);
        bobJoinsAfter = bound(bobJoinsAfter, 0, DURATION);

        _stake(alice, aliceAmt);
        vm.warp(block.timestamp + bobJoinsAfter);
        _stake(bob, bobAmt);
        vm.warp(block.timestamp + DURATION + 1 days);

        uint256 totalEarned = staking.earned(alice) + staking.earned(bob);
        assertLe(totalEarned, REWARD);
        assertLe(totalEarned, rewardToken.balanceOf(address(staking)));
    }
}
