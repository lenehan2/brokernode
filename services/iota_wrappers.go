package services

import (
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	raven "github.com/getsentry/raven-go"
	"github.com/iotaledger/giota"
	"github.com/joho/godotenv"
	"github.com/oysterprotocol/brokernode/models"
)

type ChunkTracker struct {
	ChunkCount  int
	ElapsedTime time.Duration
}

type PowJob struct {
	Chunks         []models.DataMap
	BroadcastNodes []string
}

type PowChannel struct {
	ChannelID     string
	ChunkTrackers *[]ChunkTracker
	Channel       chan PowJob
}

type IotaService struct {
	SendChunksToChannel            SendChunksToChannel
	VerifyChunkMessagesMatchRecord VerifyChunkMessagesMatchRecord
	VerifyChunksMatchRecord        VerifyChunksMatchRecord
	ChunksMatch                    ChunksMatch
}

type SendChunksToChannel func([]models.DataMap, *models.ChunkChannel)
type VerifyChunkMessagesMatchRecord func([]models.DataMap) (filteredChunks FilteredChunk, err error)
type VerifyChunksMatchRecord func([]models.DataMap, bool) (filteredChunks FilteredChunk, err error)
type ChunksMatch func(giota.Transaction, models.DataMap, bool) bool

type FilteredChunk struct {
	MatchesTangle      []models.DataMap
	DoesNotMatchTangle []models.DataMap
	NotAttached        []models.DataMap
}

// Things below are copied from the giota lib since they are not public.
// https://github.com/iotaledger/giota/blob/master/transfer.go#L322
const (
	maxTimestampTrytes = "MMMMMMMMM"
)

var (
	// PowProcs is number of concurrent processes (default is NumCPU()-1)
	PowProcs    int
	IotaWrapper IotaService
	//This mutex was added by us.
	mutex        = &sync.Mutex{}
	seed         giota.Trytes
	minDepth     = int64(giota.DefaultNumberOfWalks)
	minWeightMag = int64(9)
	bestPow      giota.PowFunc
	powName      string
	Channel      = map[string]PowChannel{}
	wg           sync.WaitGroup
	api          *giota.API
)

func init() {

	// Load ENV variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
		raven.CaptureError(err, nil)
	}

	host_ip := os.Getenv("HOST_IP")
	if host_ip == "" {
		log.Println("No IRI host given")
		raven.CaptureError(err, nil)
		// panic("Invalid IRI host: Check the .env file for HOST_IP")
	}

	provider := "http://" + host_ip + ":14265"

	api = giota.NewAPI(provider, nil)

	seed = "OYSTERPRLOYSTERPRLOYSTERPRLOYSTERPRLOYSTERPRLOYSTERPRLOYSTERPRLOYSTERPRLOYSTERPRL"

	powName, bestPow = giota.GetBestPoW()

	IotaWrapper = IotaService{
		SendChunksToChannel:            sendChunksToChannel,
		VerifyChunkMessagesMatchRecord: verifyChunkMessagesMatchRecord,
		VerifyChunksMatchRecord:        verifyChunksMatchRecord,
		ChunksMatch:                    chunksMatch,
	}

	PowProcs = runtime.NumCPU()
	if PowProcs != 1 {
		PowProcs--
	}

	//makeFakeChunks()

	channels := []models.ChunkChannel{}

	wg.Add(1)
	go func(channels *[]models.ChunkChannel, err *error) {
		defer wg.Done()
		*channels, *err = models.MakeChannels(PowProcs)
	}(&channels, &err)

	wg.Wait()

	for _, channel := range channels {

		chunkTracker := make([]ChunkTracker, 0)

		Channel[channel.ChannelID] = PowChannel{
			ChannelID:     channel.ChannelID,
			Channel:       make(chan PowJob),
			ChunkTrackers: &chunkTracker,
		}

		// start the worker
		go PowWorker(Channel[channel.ChannelID].Channel, channel.ChannelID, err)
	}
}

func makeFakeChunks() {

	dataMaps := []models.DataMap{}

	models.BuildDataMaps("GENHASH2", 49000)

	_ = models.DB.RawQuery("SELECT * from data_maps").All(&dataMaps)

	for i := 0; i < len(dataMaps); i++ {
		dataMaps[i].Address = models.RandSeq(81)
		dataMaps[i].Message = "TESTMESSAGE"
		dataMaps[i].Status = models.Unassigned

		models.DB.ValidateAndSave(&dataMaps[i])
	}
}

func PowWorker(jobQueue <-chan PowJob, channelID string, err error) {
	for powJobRequest := range jobQueue {
		// this is where we would call methods to deal with each job request
		fmt.Println("PowWorker: Starting")

		startTime := time.Now()

		transfersArray := make([]giota.Transfer, len(powJobRequest.Chunks))

		for i, chunk := range powJobRequest.Chunks {
			transfersArray[i].Address = giota.Address(chunk.Address)
			transfersArray[i].Value = int64(0)
			transfersArray[i].Message = giota.Trytes(chunk.Message)
			transfersArray[i].Tag = giota.Trytes("OYSTERGOLANG")
		}

		bdl, err := giota.PrepareTransfers(api, seed, transfersArray, nil, "", 1)

		if err != nil {
			raven.CaptureError(err, nil)
		}

		transactions := []giota.Transaction(bdl)

		transactionsToApprove, err := api.GetTransactionsToApprove(minDepth, giota.DefaultNumberOfWalks, "")
		if err != nil {
			raven.CaptureError(err, nil)
		}

		err = doPowAndBroadcast(
			transactionsToApprove.BranchTransaction,
			transactionsToApprove.TrunkTransaction,
			minDepth,
			transactions,
			minWeightMag,
			bestPow,
			powJobRequest.BroadcastNodes)

		channelToChange := Channel[channelID]

		channelInDB := models.ChunkChannel{}
		models.DB.RawQuery("SELECT * from chunk_channels where channel_id = ?", channelID).First(&channelInDB)
		channelInDB.ChunksProcessed += len(powJobRequest.Chunks)
		models.DB.ValidateAndSave(&channelInDB)

		fmt.Println("PowWorker: Leaving")
		TrackProcessingTime(startTime, len(powJobRequest.Chunks), &channelToChange)
	}
}

func TrackProcessingTime(startTime time.Time, numChunks int, channel *PowChannel) {

	*(channel.ChunkTrackers) = append(*(channel.ChunkTrackers), ChunkTracker{
		ChunkCount:  numChunks,
		ElapsedTime: time.Since(startTime),
	})

	if len(*(channel.ChunkTrackers)) > 10 {
		*(channel.ChunkTrackers) = (*(channel.ChunkTrackers))[1:11]
	}
}

func doPowAndBroadcast(branch giota.Trytes, trunk giota.Trytes, depth int64,
	trytes []giota.Transaction, mwm int64, bestPow giota.PowFunc, broadcastNodes []string) error {

	//defer oysterUtils.TimeTrack(time.Now(), "doPow_using_" + powName, analytics.NewProperties().
	//	Set("addresses", oysterUtils.MapTransactionsToAddrs(trytes)))

	var prev giota.Trytes
	var err error

	for i := len(trytes) - 1; i >= 0; i-- {
		switch {
		case i == len(trytes)-1:
			trytes[i].TrunkTransaction = trunk
			trytes[i].BranchTransaction = branch
		default:
			trytes[i].TrunkTransaction = prev
			trytes[i].BranchTransaction = trunk
		}

		timestamp := giota.Int2Trits(time.Now().UnixNano()/1000000, giota.TimestampTrinarySize).Trytes()
		trytes[i].AttachmentTimestamp = timestamp
		trytes[i].AttachmentTimestampLowerBound = ""
		trytes[i].AttachmentTimestampUpperBound = maxTimestampTrytes

		// We customized this to lock here.
		mutex.Lock()
		trytes[i].Nonce, err = bestPow(trytes[i].Trytes(), int(mwm))
		mutex.Unlock()

		if err != nil {
			raven.CaptureError(err, nil)
			return err
		}

		prev = trytes[i].Hash()
	}

	go func(trytes []giota.Transaction) {

		err = api.BroadcastTransactions(trytes)

		if err != nil {

			// Async log
			//go oysterUtils.SegmentClient.Enqueue(analytics.Track{
			//	Event:  "broadcast_fail_redoing_pow",
			//	UserId: oysterUtils.GetLocalIP(),
			//	Properties: analytics.NewProperties().
			//		Set("addresses", oysterUtils.MapTransactionsToAddrs(trytes)),
			//})

			fmt.Println(err)
			raven.CaptureError(err, nil)
		} else {

			fmt.Println("Broadcast Success!")

			/*
				TODO do we need this??
			*/
			//go BroadcastTxs(&trytes, broadcastNodes)

			//go oysterUtils.SegmentClient.Enqueue(analytics.Track{
			//	Event:  "broadcast_success",
			//	UserId: oysterUtils.GetLocalIP(),
			//	Properties: analytics.NewProperties().
			//		Set("addresses", oysterUtils.MapTransactionsToAddrs(trytes)),
			//})
		}
	}(trytes)

	return nil
}

func sendChunksToChannel(chunks []models.DataMap, channel *models.ChunkChannel) {

	for _, chunk := range chunks {
		chunk.Status = models.Unverified
		models.DB.ValidateAndSave(&chunk)
	}

	channel.EstReadyTime = SetEstimatedReadyTime(Channel[channel.ChannelID], len(chunks))
	models.DB.ValidateAndSave(channel)

	powJob := PowJob{
		Chunks:         chunks,
		BroadcastNodes: make([]string, 1),
	}

	Channel[channel.ChannelID].Channel <- powJob
}

func SetEstimatedReadyTime(channel PowChannel, numChunks int) time.Time {

	var totalTime time.Duration = 0
	chunksCount := 0

	if len(*(channel.ChunkTrackers)) != 0 {

		for _, timeRecord := range *(channel.ChunkTrackers) {
			totalTime += timeRecord.ElapsedTime
			chunksCount += timeRecord.ChunkCount
		}

		avgTimePerChunk := int(totalTime) / chunksCount
		expectedDelay := int(math.Floor((float64(avgTimePerChunk * numChunks))))

		return time.Now().Add(time.Duration(expectedDelay))
	} else {

		// The application just started, we don't have any data yet,
		// so just set est_ready_time to 10 seconds from now

		/*
			TODO:  get a more precise estimate of what this default should be
		*/
		return time.Now().Add(10 * time.Second)
	}
}

func verifyChunkMessagesMatchRecord(chunks []models.DataMap) (filteredChunks FilteredChunk, err error) {
	filteredChunks, err = verifyChunksMatchRecord(chunks, false)
	return filteredChunks, err
}

func verifyChunksMatchRecord(chunks []models.DataMap, checkChunkAndBranch bool) (filteredChunks FilteredChunk, err error) {

	addresses := make([]giota.Address, 0, len(chunks))

	for _, chunk := range chunks {
		// if a chunk did not match the tangle in verify_data_maps
		// we mark it as "Error" and there is no reason to check the tangle
		// for it again while its status is still in an Error state

		// this will cause this chunk to automatically get added to 'NotAttached' array
		// and send to the channels
		if chunk.Status != models.Error {
			addresses = append(addresses, giota.Address(chunk.Address))
		}
	}

	request := giota.FindTransactionsRequest{
		Command:   "findTransactions",
		Addresses: addresses,
	}

	response, err := api.FindTransactions(&request)

	if err != nil {
		raven.CaptureError(err, nil)
		return filteredChunks, err
	}

	filteredChunks = FilteredChunk{}

	if response != nil && len(response.Hashes) > 0 {
		trytesArray, err := api.GetTrytes(response.Hashes)
		if err != nil {
			raven.CaptureError(err, nil)
			return filteredChunks, err
		}

		transactionObjects := map[giota.Address][]giota.Transaction{}

		for _, txObject := range trytesArray.Trytes {
			transactionObjects[txObject.Address] = append(transactionObjects[txObject.Address], txObject)
		}

		for _, chunk := range chunks {

			if _, ok := transactionObjects[giota.Address(chunk.Address)]; ok {
				matchFound := false
				for _, txObject := range transactionObjects[giota.Address(chunk.Address)] {
					if chunksMatch(txObject, chunk, checkChunkAndBranch) {
						matchFound = true
						break
					}
				}
				if matchFound {
					filteredChunks.MatchesTangle = append(filteredChunks.MatchesTangle, chunk)
				} else {
					filteredChunks.DoesNotMatchTangle = append(filteredChunks.DoesNotMatchTangle, chunk)
				}
			} else {
				filteredChunks.NotAttached = append(filteredChunks.NotAttached, chunk)
			}
		}
	} else if len(response.Hashes) == 0 {
		filteredChunks.NotAttached = chunks
	}
	return filteredChunks, err
}

func chunksMatch(chunkOnTangle giota.Transaction, chunkOnRecord models.DataMap, checkBranchAndTrunk bool) bool {

	if checkBranchAndTrunk == false &&
		strings.Contains(fmt.Sprint(chunkOnTangle.SignatureMessageFragment), chunkOnRecord.Message) == true {

		return true

	} else if strings.Contains(fmt.Sprint(chunkOnTangle.SignatureMessageFragment), chunkOnRecord.Message) == true &&
		strings.Contains(fmt.Sprint(chunkOnTangle.TrunkTransaction), chunkOnRecord.TrunkTx) &&
		strings.Contains(fmt.Sprint(chunkOnTangle.BranchTransaction), chunkOnRecord.BranchTx) {

		return true

	} else {

		return false
	}
}
