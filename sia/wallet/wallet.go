package wallet

import (
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

// Wallet holds your coins, manages privacy, outputs, ect. The balance reported
// ignores outputs you've already spent even if they haven't made it into the
// blockchain yet.
//
// The spentCounter is used to indicate which transactions have been spent but
// have not appeared in the blockchain. It's used as an int for an easy reset.
// Each transaction also has a spent counter. If the transaction's spent
// counter is equal to the wallet's spent counter, then the transaction has
// been spent since the last reset. Upon reset, the wallet's spent counter is
// incremented, which means all transactions will no longer register as having
// been spent since the last reset.
//
// Wallet.transactions is a list of transactions that are currently being built
// within the wallet. The transactionCounter ensures that each
// transaction-in-progress gets a unique ID.
type Wallet struct {
	state      *consensus.State
	prevHeight consensus.BlockHeight // TODO: This will deprecate when we switch to state subscriptions.

	saveFilename string

	spentCounter        int
	addresses           map[consensus.CoinAddress]*spendableAddress
	timelockedAddresses map[consensus.BlockHeight][]*spendableAddress

	transactionCounter int
	transactions       map[string]*openTransaction

	mu sync.RWMutex
}

// New creates a new wallet, loading any known addresses from the input file
// name and then using the file to save in the future.
func New(state *consensus.State, filename string) (w *Wallet, err error) {
	w = &Wallet{
		state: state,

		saveFilename: filename,

		spentCounter:                 1,
		spendableAddresses:           make(map[consensus.CoinAddress]*spendableAddress),
		timelockedSpendableAddresses: make(map[consensus.BlockHeight][]*spendableAddress),

		transactions: make(map[string]*openTransaction),
	}

	err = w.Load(filename)
	if err != nil {
		return
	}

	return
}

// Info implements the core.Wallet interface.
func (w *Wallet) WalletInfo() (status components.WalletInfo, err error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	status = components.WalletInfo{
		Balance:      w.Balance(false),
		FullBalance:  w.Balance(true),
		NumAddresses: len(w.spendableAddresses),
	}

	return
}

// SpendCoins creates a transaction sending 'amount' to 'dest', and
// allocateding 'minerFee' as a miner fee. The transaction is submitted to the
// miner pool, but is also returned.
func (w *Wallet) SpendCoins(amount consensus.Currency, dest consensus.CoinAddress) (t consensus.Transaction, err error) {
	// Create and send the transaction.
	minerFee := consensus.Currency(10) // TODO: wallet supplied miner fee
	output := consensus.Output{
		Value:     amount,
		SpendHash: dest,
	}
	id, err := w.RegisterTransaction(t)
	if err != nil {
		return
	}
	err = w.FundTransaction(id, amount+minerFee)
	if err != nil {
		return
	}
	err = w.AddMinerFee(id, minerFee)
	if err != nil {
		return
	}
	err = w.AddOutput(id, output)
	if err != nil {
		return
	}
	t, err = w.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = w.state.AcceptTransaction(t)
	return
}
