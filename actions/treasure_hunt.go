package actions

import (
	"github.com/gobuffalo/buffalo"
	"github.com/oysterprotocol/brokernode/models"
	"github.com/oysterprotocol/brokernode/utils"
)

type TreasureHuntResource struct {
	buffalo.Resource
}

type treasureHuntCreateReq struct {
	ReceiverEthAddr string `json:"receiverEthAddr"`
	GenesisHash string `json:"genesisHash"`
	SectorIdx string `json:"sectorIdx"`
	NumberChunks string `json:"numberChunks"`
	EthAddr string `json:"ethAddr"`
	EthKey string `json:"ethKey"`
}

type treasureHuntCreateRes struct {
	TreasureHunt models.TreasureHunt `json:"id"`
}

func (usr *TreasureHuntResource) Create(c buffalo.Context) error {
	req := treasureHuntCreateReq{}
	oyster_utils.ParseReqBody(c.Request(), &req)

	w := models.TreasureHunt{
		ReceiverEthAddr: req.ReceiverEthAddr,
		GenesisHash: req.GenesisHash,
		SectorIdx: req.SectorIdx,
		NumberChunks: req.NumberChunks,
		EthAddr: req.EthAddr,
		EthKey: req.EthKey,
	}

	res := treasureHuntCreateRes{
		TreasureHunt: w,
	}

	return c.Render(200, r.JSON(res))
}
