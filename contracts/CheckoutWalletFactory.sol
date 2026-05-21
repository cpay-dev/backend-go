// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

contract CheckoutWallet {
    address public immutable factory;

    constructor() {
        factory = msg.sender;
    }

    receive() external payable {}

    function sweep(address token, address recipient, uint256 amount) external {
        require(msg.sender == factory, "factory only");
        require(recipient != address(0), "recipient required");
        if (token == address(0)) {
            (bool ok, ) = recipient.call{value: amount}("");
            require(ok, "native sweep failed");
            return;
        }
        (bool ok, bytes memory data) = token.call(abi.encodeWithSignature("transfer(address,uint256)", recipient, amount));
        require(ok && (data.length == 0 || abi.decode(data, (bool))), "erc20 sweep failed");
    }
}

contract CheckoutWalletFactory {
    address public owner;
    address public sweeper;

    event SweeperUpdated(address indexed sweeper);
    event WalletSwept(bytes32 indexed salt, address indexed wallet, address indexed token, address recipient, uint256 amount);

    constructor(address initialSweeper) {
        require(initialSweeper != address(0), "sweeper required");
        owner = msg.sender;
        sweeper = initialSweeper;
    }

    modifier onlyOwner() {
        require(msg.sender == owner, "owner only");
        _;
    }

    modifier onlySweeper() {
        require(msg.sender == sweeper, "sweeper only");
        _;
    }

    function setSweeper(address nextSweeper) external onlyOwner {
        require(nextSweeper != address(0), "sweeper required");
        sweeper = nextSweeper;
        emit SweeperUpdated(nextSweeper);
    }

    function walletAddress(bytes32 salt) public view returns (address) {
        return address(uint160(uint256(keccak256(abi.encodePacked(
            bytes1(0xff),
            address(this),
            salt,
            keccak256(type(CheckoutWallet).creationCode)
        )))));
    }

    function deployAndSweep(bytes32 salt, address token, address recipient, uint256 amount) external onlySweeper {
        address payable wallet = payable(walletAddress(salt));
        if (wallet.code.length == 0) {
            new CheckoutWallet{salt: salt}();
        }
        CheckoutWallet(wallet).sweep(token, recipient, amount);
        emit WalletSwept(salt, wallet, token, recipient, amount);
    }
}
