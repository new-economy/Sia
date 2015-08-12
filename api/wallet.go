package api

import (
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/NebulousLabs/entropy-mnemonics"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// WalletGET contains general information about the wallet, with tags to
// support idiomatic json encodings.
type WalletGET struct {
	Encrypted bool `json:"encrypted"`
	Unlocked  bool `json:"unlocked"`

	ConfirmedSiacoinBalance     types.Currency `json:"confirmedSiacoinBalance"`
	UnconfirmedOutgoingSiacoins types.Currency `json:"unconfirmedOutgoingSiacoins"`
	UnconfirmedIncomingSiacoins types.Currency `json:"unconfirmedIncomingSiacoins"`

	SiafundBalance      types.Currency `json:"siafundBalance"`
	SiacoinClaimBalance types.Currency `json:"siacoinClaimBalance"`
}

// WalletHistoryGet contains wallet transaction history.
type WalletHistoryGET struct {
	UnconfirmedTransactions []modules.WalletTransaction `json:"unconfirmedTransactions"`
	ConfirmedTransactions   []modules.WalletTransaction `json:"confirmedTransactions"`
}

// WalletSeedGet contains the seeds used by the wallet.
type WalletSeedGET struct {
	PrimarySeed        string   `json:"primarySeed"`
	AddressesRemaining int      `json:"AddressesRemaining"`
	AllSeeds           []string `json:"allSeeds"`
}

// WalletSeedPUT contains an address returned by a PUT call to /wallet/seed.
type WalletSeedPUT struct {
	Address types.UnlockHash `json:"address"`
}

// WalletSeedPOST contains the new seed generated by a POST call to
// /wallet/seed.
type WalletSeedPOST struct {
	NewSeed string `json:"newSeed"`
}

// WalletTransactionGETid contains the transaction returned by a call to
// /wallet/transaction/$(id)
type WalletTransactionGETid struct {
	Transaction types.Transaction `json:"transaction"`
}

// WalletTransactionsGET contains the specified set of confirmed and
// unconfirmed transactions.
type WalletTransactionsGET struct {
	ConfirmedTransactions   []types.Transaction `json:"confirmedTransactions"`
	UnconfirmedTransactions []types.Transaction `json:"unconfirmedTransactions"`
}

// scanAmount scans a types.Currency from a string.
func scanAmount(amount string) (types.Currency, bool) {
	// use SetString manually to ensure that amount does not contain
	// multiple values, which would confuse fmt.Scan
	i, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return types.Currency{}, ok
	}
	return types.NewCurrency(i), true
}

// scanAddres scans a types.UnlockHash from a string.
func scanAddress(addrStr string) (addr types.UnlockHash, err error) {
	err = addr.LoadString(addrStr)
	return
}

// walletHandlerGET handles a GET request to /wallet.
func (srv *Server) walletHandlerGET(w http.ResponseWriter, req *http.Request) {
	siacoinBal, siafundBal, siaclaimBal := srv.wallet.ConfirmedBalance()
	siacoinsOut, siacoinsIn := srv.wallet.UnconfirmedBalance()
	writeJSON(w, WalletGET{
		Encrypted: srv.wallet.Encrypted(),
		Unlocked:  srv.wallet.Unlocked(),

		ConfirmedSiacoinBalance:     siacoinBal,
		UnconfirmedOutgoingSiacoins: siacoinsOut,
		UnconfirmedIncomingSiacoins: siacoinsIn,

		SiafundBalance:      siafundBal,
		SiacoinClaimBalance: siaclaimBal,
	})
}

// walletHander handles API calls to /wallet.
func (srv *Server) walletHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.walletHandlerGET(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet", http.StatusBadRequest)
	}
}

// walletCloseHandlerPUT handles a PUT request to /wallet/close.
func (srv *Server) walletCloseHandlerPUT(w http.ResponseWriter, req *http.Request) {
	err := srv.wallet.Close()
	if err == nil {
		writeSuccess(w)
	} else {
		writeError(w, err.Error(), http.StatusBadRequest)
	}
}

// walletCloseHanlder handles API calls to /wallet/close.
func (srv *Server) walletCloseHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "PUT" {
		srv.walletCloseHandlerPUT(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/close", http.StatusBadRequest)
	}
}

// walletHistoryHandlerGET handles a GET request to /wallet/history.
func (srv *Server) walletHistoryHandlerGET(w http.ResponseWriter, req *http.Request) {
	start, err := strconv.Atoi(req.FormValue("startHeight"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	end, err := strconv.Atoi(req.FormValue("endHeight"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	confirmedHistory, err := srv.wallet.History(types.BlockHeight(start), types.BlockHeight(end))
	if err != nil {
		writeError(w, "error after call to /wallet/history: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletHistoryGET{
		UnconfirmedTransactions: srv.wallet.UnconfirmedHistory(),
		ConfirmedTransactions:   confirmedHistory,
	})
}

// walletHistoryHandlerGETaddr handles a GET request to
// /wallet/history/$(addr).
func (srv *Server) walletHistoryHandlerGETaddr(w http.ResponseWriter, req *http.Request, addr types.UnlockHash) {
	addrHistory, err := srv.wallet.AddressHistory(addr)
	if err != nil {
		writeError(w, "error after call to /wallet/history/$(addr): "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, addrHistory)
}

// walletHistoryHandler handles all API calls to /wallet/history
func (srv *Server) walletHistoryHandler(w http.ResponseWriter, req *http.Request) {
	// Check for a vanilla call to /wallet/history.
	if req.URL.Path == "/wallet/history" && req.Method == "GET" || req.Method == "" {
		srv.walletHistoryHandlerGET(w, req)
	}

	// The only remaining possibility is a GET call to /wallet/history/$(addr);
	// check that the method is correct.
	if req.Method != "GET" && req.Method != "" {
		writeError(w, "unrecognized method in call to /wallet/history", http.StatusBadRequest)
		return
	}

	// Parse the address from the url and call the GETaddr Handler.
	jsonAddr := "\"" + strings.TrimPrefix(req.URL.Path, "/wallet/history/") + "\""
	var addr types.UnlockHash
	err := addr.UnmarshalJSON([]byte(jsonAddr))
	if err != nil {
		writeError(w, "error after call to /wallet/history: "+err.Error(), http.StatusBadRequest)
		return
	}
	srv.walletHistoryHandlerGETaddr(w, req, addr)
}

// walletSeedHandlerGET handles a GET request to /wallet/seed.
func (srv *Server) walletSeedHandlerGET(w http.ResponseWriter, req *http.Request) {
	dictionary := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictionary == "" {
		dictionary = mnemonics.English
	}

	// Get the primary seed information.
	primarySeed, progress, err := srv.wallet.PrimarySeed()
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}
	primarySeedStr, err := modules.SeedToString(primarySeed, dictionary)
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get the list of seeds known to the wallet.
	allSeeds := srv.wallet.AllSeeds()
	var allSeedsStrs []string
	for _, seed := range allSeeds {
		str, err := modules.SeedToString(seed, dictionary)
		if err != nil {
			writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
			return
		}
		allSeedsStrs = append(allSeedsStrs, str)
	}
	writeJSON(w, WalletSeedGET{
		PrimarySeed:        primarySeedStr,
		AddressesRemaining: int(modules.PublicKeysPerSeed - progress),
		AllSeeds:           allSeedsStrs,
	})
}

// walletSeedHandlerPUT handles a PUT request to /wallet/seed.
func (srv *Server) walletSeedHandlerPUT(w http.ResponseWriter, req *http.Request) {
	unlockConditions, err := srv.wallet.NextAddress()
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletSeedPUT{
		Address: unlockConditions.UnlockHash(),
	})
}

// walletSeedHandlerPOST handles a POST request to /wallet/seed.
func (srv *Server) walletSeedHandlerPOST(w http.ResponseWriter, req *http.Request) {
	// Fetch the new seed.
	encryptionKey := crypto.TwofishKey(crypto.HashObject(req.FormValue("encryptionKey")))
	did := mnemonics.DictionaryID(req.FormValue("dictionary"))
	seed, err := srv.wallet.NewPrimarySeed(encryptionKey)
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}

	seedStr, err := modules.SeedToString(seed, did)
	if err != nil {
		writeError(w, "error after call to /wallet/seed: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletSeedPOST{
		NewSeed: seedStr,
	})
}

// walletSeedHandler handles API calls to /wallet/seed.
func (srv *Server) walletSeedHandler(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET", "":
		srv.walletSeedHandlerGET(w, req)
	case "PUT":
		srv.walletSeedHandlerPUT(w, req)
	case "POST":
		srv.walletSeedHandlerPOST(w, req)
	default:
		writeError(w, "unrecognized method when calling /wallet/seed", http.StatusBadRequest)
	}
}

// walletSiacoinsHandlerPUT handles a PUT request to /wallet/siacoins.
func (srv *Server) walletSiacoinsHandlerPUT(w http.ResponseWriter, req *http.Request) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, "could not read 'amount' from PUT call to /wallet/siacoins", http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, "error after call to /wallet/siacoins: "+err.Error(), http.StatusBadRequest)
		return
	}

	_, err = srv.wallet.SendSiacoins(amount, dest)
	if err != nil {
		writeError(w, "error after call to /wallet/siacoins: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeSuccess(w)
}

// walletSiacoinsHandler handles API calls to /wallet/siacoins.
func (srv *Server) walletSiacoinsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "PUT" {
		srv.walletSiacoinsHandlerPUT(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/siacoins", http.StatusBadRequest)
	}
}

// walletSiafundsHandlerPUT handles a PUT request to /wallet/siafunds.
func (srv *Server) walletSiafundsHandlerPUT(w http.ResponseWriter, req *http.Request) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		writeError(w, "could not read 'amount' from PUT call to /wallet/siafunds", http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		writeError(w, "error after call to /wallet/siafunds: "+err.Error(), http.StatusBadRequest)
		return
	}

	_, err = srv.wallet.SendSiafunds(amount, dest)
	if err != nil {
		writeError(w, "error after call to /wallet/siafunds: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeSuccess(w)
}

// walletSiafundsHandler handles API calls to /wallet/siafunds.
func (srv *Server) walletSiafundsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "PUT" {
		srv.walletSiafundsHandlerPUT(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/siafunds", http.StatusBadRequest)
	}
}

// walletTransactionHandlerGETid handles a GET call to
// /wallet/transaction/$(id).
func (srv *Server) walletTransactionHandlerGETid(w http.ResponseWriter, req *http.Request, id types.TransactionID) {
	txn, ok := srv.wallet.Transaction(id)
	if !ok {
		writeError(w, "error when calling /wallet/transaction/$(id): transaction not found", http.StatusBadRequest)
		return
	}
	writeJSON(w, WalletTransactionGETid{
		Transaction: txn,
	})
}

// walletTransactionHandler handles API calls to /wallet/transaction.
func (srv *Server) walletTransactionHandler(w http.ResponseWriter, req *http.Request) {
	// GET is the only supported method.
	if req.Method != "" && req.Method != "GET" {
		writeError(w, "unrecognized method when calling /wallet/transaction", http.StatusBadRequest)
		return
	}

	// Parse the id from the url.
	var id types.TransactionID
	jsonID := "\"" + strings.TrimPrefix(req.URL.Path, "/wallet/transaction/") + "\""
	err := id.UnmarshalJSON([]byte(jsonID))
	if err != nil {
		writeError(w, "error after call to /wallet/history: "+err.Error(), http.StatusBadRequest)
		return
	}
	srv.walletTransactionHandlerGETid(w, req, id)
}

// walletTransactionsHandlerGET handles a GET call to /wallet/transactions.
func (srv *Server) walletTransactionsHandlerGET(w http.ResponseWriter, req *http.Request) {
	// Get the start and end blocks.
	start, err := strconv.Atoi(req.FormValue("startHeight"))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	end, err := strconv.Atoi(req.FormValue("endHeight"))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	confirmedTxns, err := srv.wallet.Transactions(types.BlockHeight(start), types.BlockHeight(end))
	if err != nil {
		writeError(w, "error after call to /wallet/transactions: "+err.Error(), http.StatusBadRequest)
		return
	}
	unconfirmedTxns := srv.wallet.UnconfirmedTransactions()

	writeJSON(w, WalletTransactionsGET{
		ConfirmedTransactions:   confirmedTxns,
		UnconfirmedTransactions: unconfirmedTxns,
	})
}

// walletTransactionsHandler handles API calls to /wallet/transactions.
func (srv *Server) walletTransactionsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.walletTransactionsHandlerGET(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/transactions", http.StatusBadRequest)
	}
}

// walletUnlockHandlerPUT handles a PUT call to /wallet/unlock.
func (srv *Server) walletUnlockHandlerPUT(w http.ResponseWriter, req *http.Request) {
	encryptionKey := crypto.TwofishKey(crypto.HashObject(req.FormValue("encryptionKey")))
	err := srv.wallet.Unlock(encryptionKey)
	if err != nil {
		writeError(w, "error when calling /wallet/unlock: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// walletUnlockHandler handles API calls to /wallet/unlock.
func (srv *Server) walletUnlockHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "PUT" {
		srv.walletUnlockHandlerPUT(w, req)
	} else {
		writeError(w, "unrecognized method when calling /wallet/unlock", http.StatusBadRequest)
	}
}
