package main

import (
	"encoding/json"
	"time"

	"fmt"
	"strings"

	"github.com/ethereum/hive/hivesim"
	"github.com/ethereum/hive/optimism"
)

type testSpec struct {
	Name  string
	About string
	Run   func(*TestEnv)
}

var tests = []testSpec{
	{Name: "http/BalanceAndNonceAt", Run: balanceAndNonceAtTest},
	{Name: "http/CanonicalChain", Run: canonicalChainTest},
	{Name: "http/CodeAt", Run: CodeAtTest},
	{Name: "http/ContractDeployment", Run: deployContractTest},
	{Name: "http/ContractDeploymentOutOfGas", Run: deployContractOutOfGasTest},
	{Name: "http/EstimateGas", Run: estimateGasTest},
	{Name: "http/GenesisBlockByHash", Run: genesisBlockByHashTest},
	{Name: "http/GenesisBlockByNumber", Run: genesisBlockByNumberTest},
	{Name: "http/GenesisHeaderByHash", Run: genesisHeaderByHashTest},
	{Name: "http/GenesisHeaderByNumber", Run: genesisHeaderByNumberTest},
	{Name: "http/Receipt", Run: receiptTest},
	{Name: "http/SyncProgress", Run: syncProgressTest},
	{Name: "http/TransactionCount", Run: transactionCountTest},
	{Name: "http/TransactionInBlock", Run: transactionInBlockTest},
	{Name: "http/TransactionReceipt", Run: TransactionReceiptTest},

	// HTTP ABI tests.
	{Name: "http/ABICall", Run: callContractTest},
	{Name: "http/ABITransact", Run: transactContractTest},

	// WebSocket ABI tests.
	{Name: "ws/ABICall", Run: callContractTest},
	{Name: "ws/ABITransact", Run: transactContractTest},

	// WebSocket subscription tests.
	{Name: "ws/NewHeadSubscription", Run: newHeadSubscriptionTest},
	{Name: "ws/LogSubscription", Run: logSubscriptionTest},
	{Name: "ws/TransactionInBlockSubscription", Run: transactionInBlockSubscriptionTest},

	// WebSocket RPC tests.
	{Name: "ws/BalanceAndNonceAt", Run: balanceAndNonceAtTest},
	{Name: "ws/CanonicalChain", Run: canonicalChainTest},
	{Name: "ws/CodeAt", Run: CodeAtTest},
	{Name: "ws/ContractDeployment", Run: deployContractTest},
	{Name: "ws/ContractDeploymentOutOfGas", Run: deployContractOutOfGasTest},
	{Name: "ws/EstimateGas", Run: estimateGasTest},
	{Name: "ws/GenesisBlockByHash", Run: genesisBlockByHashTest},
	{Name: "ws/GenesisBlockByNumber", Run: genesisBlockByNumberTest},
	{Name: "ws/GenesisHeaderByHash", Run: genesisHeaderByHashTest},
	{Name: "ws/GenesisHeaderByNumber", Run: genesisHeaderByNumberTest},
	{Name: "ws/Receipt", Run: receiptTest},
	{Name: "ws/SyncProgress", Run: syncProgressTest},
	{Name: "ws/TransactionCount", Run: transactionCountTest},
	{Name: "ws/TransactionInBlock", Run: transactionInBlockTest},
	{Name: "ws/TransactionReceipt", Run: TransactionReceiptTest},
}

func main() {
	suite := hivesim.Suite{
		Name: "optimism rpc",
		Description: `
The RPC test suite runs a set of RPC related tests against a running node. It tests
several real-world scenarios such as sending value transactions, deploying a contract or
interacting with one.`[1:],
	}

	// Add tests for full nodes.
	suite.Add(&hivesim.TestSpec{
		Name:        "client launch",
		Description: `This test launches the client and collects its logs.`,
		Run:         func(t *hivesim.T) { runAllTests(t) },
	})

	sim := hivesim.New()
	hivesim.MustRunSuite(sim, suite)
}

// runAllTests runs the tests against a client instance.
// Most tests simply wait for tx inclusion in a block so we can run many tests concurrently.
func runAllTests(t *hivesim.T) {
	handleErr := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	d := optimism.NewDevnet(t)

	d.InitContracts()
	d.InitHardhatDeployConfig()
	d.InitL1Hardhat()
	d.AddEth1() // l1 eth1 node is required for l2 config init
	d.WaitUpEth1(0, time.Second*10)
	// deploy contracts
	d.DeployL1Hardhat()

	d.InitL2Hardhat()
	d.AddOpL2() // l2 engine is required for rollup config init
	d.WaitUpOpL2Engine(0, time.Second*10)
	d.InitRollupHardhat()

	// sequencer stack, on top of first eth1 node
	d.AddOpNode(0, 0)
	d.AddOpBatcher(0, 0, 0)
	// proposer does not need to run for L2 to be stable
	//d.AddOpProposer(0, 0, 0)

	// verifier stack (optional)
	//d.AddOpL2()
	//d.AddOpNode(0, 1)  // can use the same eth1 node

	c := d.GetOpL2Engine(0).Client

	vault := newVault()
	genesis, err := json.Marshal(d.L1Cfg)
	handleErr(err)

	s := newSemaphore(40)
	for _, test := range tests {
		test := test
		s.get()
		go func() {
			defer s.put()
			t.Run(hivesim.TestSpec{
				Name:        fmt.Sprintf("%s (%s)", test.Name, "ops-l2"),
				Description: test.About,
				Run: func(t *hivesim.T) {
					switch test.Name[:strings.IndexByte(test.Name, '/')] {
					case "http":
						runHTTP(t, c, vault, genesis, test.Run)
					case "ws":
						runWS(t, c, vault, genesis, test.Run)
					default:
						panic("bad test prefix in name " + test.Name)
					}
				},
			})
			// TODO: debug re-org issue and remove
			time.Sleep(5 * time.Second)
		}()
	}
	s.drain()
}

type semaphore chan struct{}

func newSemaphore(n int) semaphore {
	s := make(semaphore, n)
	for i := 0; i < n; i++ {
		s <- struct{}{}
	}
	return s
}

func (s semaphore) get() { <-s }
func (s semaphore) put() { s <- struct{}{} }

func (s semaphore) drain() {
	for i := 0; i < cap(s); i++ {
		<-s
	}
}
