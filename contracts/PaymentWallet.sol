// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IERC20 {
    function balanceOf(address) external view returns (uint256);
    function transfer(address, uint256) external returns (bool);
}

/// @title PaymentWallet
/// @notice Counterfactual wallet for cpay.dev merchants.
///         Anyone can trigger withdrawals, but funds always go to the immutable owner.
contract PaymentWallet {
    address public immutable owner;

    event Withdrawn(address indexed token, uint256 amount);
    event WithdrawnNative(uint256 amount);

    constructor(address _owner) {
        owner = _owner;
    }

    /// @notice Withdraw full ERC-20 token balance to owner. Permissionless.
    function withdraw(address token) external {
        uint256 bal = IERC20(token).balanceOf(address(this));
        require(bal > 0, "no balance");
        require(IERC20(token).transfer(owner, bal), "transfer failed");
        emit Withdrawn(token, bal);
    }

    /// @notice Withdraw full native token balance to owner. Permissionless.
    function withdrawNative() external {
        uint256 bal = address(this).balance;
        require(bal > 0, "no balance");
        (bool ok, ) = owner.call{value: bal}("");
        require(ok, "native transfer failed");
        emit WithdrawnNative(bal);
    }

    /// @notice Escape hatch: arbitrary call, restricted to owner only.
    function execute(address to, uint256 value, bytes calldata data) external returns (bytes memory) {
        require(msg.sender == owner, "not owner");
        (bool ok, bytes memory result) = to.call{value: value}(data);
        require(ok, "call failed");
        return result;
    }

    receive() external payable {}
}
