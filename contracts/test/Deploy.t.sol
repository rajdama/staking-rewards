// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Test} from "forge-std/Test.sol";
import {Deploy} from "../script/Deploy.s.sol";
import {Renew} from "../script/Renew.s.sol";
import {StakingRewards} from "../src/StakingRewards.sol";
import {FaucetToken} from "../src/FaucetToken.sol";

/// @notice Runs the real deployment and renewal scripts against the local test chain.
contract DeployScriptTest is Test {
    function test_DeployAndRenew() public {
        // Anvil's well-known first test key. Never use it on a real network.
        uint256 pk = 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80;
        vm.setEnv("PRIVATE_KEY", vm.toString(bytes32(pk)));

        (StakingRewards staking, FaucetToken stakeToken, FaucetToken rewardToken) = new Deploy().run();

        assertEq(address(staking.stakingToken()), address(stakeToken));
        assertEq(address(staking.rewardsToken()), address(rewardToken));
        assertEq(staking.owner(), vm.addr(pk));
        assertEq(rewardToken.balanceOf(address(staking)), 1_000e18);
        assertGt(staking.rewardRate(), 0);

        vm.warp(block.timestamp + 7 days);
        new Renew().run();
        assertEq(rewardToken.balanceOf(address(staking)), 2_000e18);
        assertEq(staking.finishAt(), block.timestamp + 7 days);
    }
}
