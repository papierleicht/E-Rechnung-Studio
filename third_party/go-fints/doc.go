// Package fints implements a bank-neutral FinTS 3.0 PIN/TAN client.
//
// It negotiates accounts, transaction formats, TAN mechanisms, and SEPA credit
// transfers from bank and user parameter data. The package performs network
// operations only when a Client method is called; NewClient validates and
// copies configuration without connecting to a bank.
//
// Applications are responsible for protecting credentials, persisting the
// synchronized system ID when desired, presenting TAN challenges, and
// confirming payment details before submitting a SEPA credit transfer.
package fints
