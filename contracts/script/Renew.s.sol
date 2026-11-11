// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console} from "forge-std/Script.sol";
import {StakingRewards} from "../src/StakingRewards.sol";
import {FaucetToken} from "../src/FaucetToken.sol";

/// @notice Tops up the reward pool and starts the next period, so the demo never runs dry.
///         Reads the staking address from deployments/<chainId>.json.
///
/// Usage:
///   PRIVATE_KEY=0x... forge script script/Renew.s.sol --rpc-url sepolia --broadcast
contract Renew is Script {
    uint256 internal constant REWARD_PER_PERIOD = 1_000e18;

    function run() external {
        uint256 pk = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(pk);

        string memory json = vm.readFile(string.concat("./deployments/", vm.toString(block.chainid), ".json"));
        StakingRewards staking = StakingRewards(vm.parseJsonAddress(json, ".staking"));
        FaucetToken rewardToken = FaucetToken(address(staking.rewardsToken()));

        vm.startBroadcast(pk);
        rewardToken.mint(deployer, REWARD_PER_PERIOD);
        rewardToken.approve(address(staking), REWARD_PER_PERIOD);
        staking.notifyRewardAmount(REWARD_PER_PERIOD);
        vm.stopBroadcast();

        console.log("Renewed. Period now ends at", staking.finishAt());
    }
}
