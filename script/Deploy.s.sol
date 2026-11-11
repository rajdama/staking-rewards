// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console} from "forge-std/Script.sol";
import {StakingRewards} from "../src/StakingRewards.sol";
import {MockERC20} from "../test/mocks/MockERC20.sol";

/// @notice Deploys two test tokens + the staking contract and starts a 7-day reward period.
/// Usage:
///   forge script script/Deploy.s.sol --rpc-url $SEPOLIA_RPC_URL --account <keystore-name> --broadcast
contract Deploy is Script {
    function run() external {
        vm.startBroadcast();

        MockERC20 stakeToken = new MockERC20("Stake Token", "STK");
        MockERC20 rewardToken = new MockERC20("Reward Token", "RWD");
        StakingRewards staking = new StakingRewards(address(stakeToken), address(rewardToken), 7 days);

        uint256 reward = 1_000e18;
        rewardToken.mint(msg.sender, reward);
        rewardToken.approve(address(staking), reward);
        staking.notifyRewardAmount(reward);

        vm.stopBroadcast();

        console.log("StakeToken:     ", address(stakeToken));
        console.log("RewardToken:    ", address(rewardToken));
        console.log("StakingRewards: ", address(staking));
    }
}
