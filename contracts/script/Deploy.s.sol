// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console} from "forge-std/Script.sol";
import {StakingRewards} from "../src/StakingRewards.sol";
import {FaucetToken} from "../src/FaucetToken.sol";

/// @notice Deploys the test tokens and staking contract, starts the first reward period,
///         and writes the addresses to deployments/<chainId>.json.
///
/// Usage:
///   PRIVATE_KEY=0x... forge script script/Deploy.s.sol --rpc-url sepolia --broadcast --verify
contract Deploy is Script {
    uint256 internal constant REWARD_DURATION = 7 days;
    uint256 internal constant REWARD_PER_PERIOD = 1_000e18;
    uint256 internal constant FAUCET_AMOUNT = 1_000e18;

    function run() external returns (StakingRewards staking, FaucetToken stakeToken, FaucetToken rewardToken) {
        uint256 pk = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(pk);

        vm.startBroadcast(pk);
        stakeToken = new FaucetToken("Stake Token", "STK", FAUCET_AMOUNT);
        rewardToken = new FaucetToken("Reward Token", "RWD", FAUCET_AMOUNT);
        staking = new StakingRewards(address(stakeToken), address(rewardToken), REWARD_DURATION);

        rewardToken.mint(deployer, REWARD_PER_PERIOD);
        rewardToken.approve(address(staking), REWARD_PER_PERIOD);
        staking.notifyRewardAmount(REWARD_PER_PERIOD);
        vm.stopBroadcast();

        string memory obj = "deployment";
        vm.serializeUint(obj, "chainId", block.chainid);
        vm.serializeUint(obj, "startBlock", block.number);
        vm.serializeAddress(obj, "stakeToken", address(stakeToken));
        vm.serializeAddress(obj, "rewardToken", address(rewardToken));
        string memory json = vm.serializeAddress(obj, "staking", address(staking));
        vm.writeJson(json, string.concat("./deployments/", vm.toString(block.chainid), ".json"));

        console.log("StakeToken:     ", address(stakeToken));
        console.log("RewardToken:    ", address(rewardToken));
        console.log("StakingRewards: ", address(staking));
    }
}
