package models

import (
	"encoding/json"
	"github.com/gobuffalo/pop"
	"github.com/gobuffalo/uuid"
	"github.com/gobuffalo/validate"
	"time"
)

type TreasureHunt struct {
	ID uuid.UUID `json:"id" db:"id"`
	ReceiverEthAddr string `json:"receiverEthAddr" db:"receiver_eth_addr"`
	GenesisHash string `json:"genesisHash" db:"genesis_hash"`
	SectorIdx string `json:"sectorIdx" db:"sector_idx"`
	NumberChunks string `json:"numberChunks" db:"number_chunks"`
	EthAddr string `json:"ethAddr" db:"eth_addr"`
	EthKey string `json:"ethKey" db:"eth_key"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// String is not required by pop and may be deleted
func (t TreasureHunt) String() string {
	jt, _ := json.Marshal(t)
	return string(jt)
}

// TreasureHunts is not required by pop and may be deleted
type TreasureHunts []TreasureHunt

// String is not required by pop and may be deleted
func (t TreasureHunts) String() string {
	jt, _ := json.Marshal(t)
	return string(jt)
}

// Validate gets run every time you call a "pop.Validate*" (pop.ValidateAndSave, pop.ValidateAndCreate, pop.ValidateAndUpdate) method.
// This method is not required and may be deleted.
func (t *TreasureHunt) Validate(tx *pop.Connection) (*validate.Errors, error) {
	return validate.NewErrors(), nil
}

// ValidateCreate gets run every time you call "pop.ValidateAndCreate" method.
// This method is not required and may be deleted.
func (t *TreasureHunt) ValidateCreate(tx *pop.Connection) (*validate.Errors, error) {
	return validate.NewErrors(), nil
}

// ValidateUpdate gets run every time you call "pop.ValidateAndUpdate" method.
// This method is not required and may be deleted.
func (t *TreasureHunt) ValidateUpdate(tx *pop.Connection) (*validate.Errors, error) {
	return validate.NewErrors(), nil
}
