-- Reset counterfactual account cache: new PaymentWallet bytecode produces different CREATE2 addresses.
TRUNCATE TABLE counterfactual_accounts;
