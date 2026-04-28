# Report: Implementation of Cancel and Query methods for Card Terminal

## Overview
Implemented the `CancelPayment` and `QueryPayment` methods in `vwork/cmd/connector/card_terminal.go` to support refund and query functionalities for Kpay, BBMSL, and HSBC terminals.

## Changes
1.  **CancelPayment**: Implemented the main dispatch function and specific handlers:
    -   `cancelKpayPayment`: Sends `VOID` command with `OrderID`.
    -   `cancelBBMSLPayment`: Sends `VOID` command with `EcrRef`.
    -   `cancelHSBCPayment`: Sends `VOID` command with `OrderID`.

2.  **QueryPayment**: Implemented the main dispatch function and specific handlers:
    -   `queryKpayPayment`: Sends `QUERY` command with `OrderID`.
    -   `queryBBMSLPayment`: Sends `QUERY` command with `EcrRef`.
    -   `queryHSBCPayment`: Sends `QUERY` command with `OrderID`.

3.  **Logic Consistency**: ensured that the `transactionID` parameter in `CancelPayment` and `QueryPayment` is treated as the original `OrderID` (or `EcrRef`), consistent with how `ProcessPayment` uses `req.OrderID`.

## Verification
-   **Compilation**: Successfully built the `connector` package using `go build`, confirming no syntax errors or type mismatches.
-   **Logic Check**: Verified that the request structures for `VOID` and `QUERY` commands match the patterns established in `ProcessPayment` and the struct definitions.

## Next Steps
-   Deploy the updated connector to a test environment with actual card terminals to verify the communication and protocol details.
-   Monitor logs for any protocol-specific errors during `VOID` or `QUERY` operations.
