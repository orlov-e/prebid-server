package yieldmo

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/prebid/openrtb/v19/openrtb2"
	"github.com/prebid/prebid-server/v2/adapters"
	"github.com/prebid/prebid-server/v2/config"
	"github.com/prebid/prebid-server/v2/errortypes"
	"github.com/prebid/prebid-server/v2/openrtb_ext"
)

type YieldmoAdapter struct {
	endpoint string
}

type ExtImpBidderYieldmo struct {
	adapters.ExtImpBidder
	Data *ExtData `json:"data,omitempty"`
}

type ExtData struct {
	PbAdslot string `json:"pbadslot"`
}

type Ext struct {
	PlacementId string `json:"placement_id"`
	Gpid        string `json:"gpid,omitempty"`
}

type ExtBid struct {
	MediaType string `json:"mediatype,omitempty"`
}

func (a *YieldmoAdapter) MakeRequests(request *openrtb2.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	var errs []error
	var adapterRequests []*adapters.RequestData

	adapterReq, errors := a.makeRequest(request)
	if adapterReq != nil {
		adapterRequests = append(adapterRequests, adapterReq)
	}
	errs = append(errs, errors...)

	return adapterRequests, errors
}

func (a *YieldmoAdapter) makeRequest(request *openrtb2.BidRequest) (*adapters.RequestData, []error) {
	var errs []error

	if err := preprocess(request); err != nil {
		errs = append(errs, err)
	}

	// Last Step
	reqJSON, err := json.Marshal(request)
	if err != nil {
		errs = append(errs, err)
		return nil, errs
	}

	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")

	return &adapters.RequestData{
		Method:  "POST",
		Uri:     a.endpoint,
		Body:    reqJSON,
		Headers: headers,
	}, errs
}

// Mutate the request to get it ready to send to yieldmo.
func preprocess(request *openrtb2.BidRequest) error {
	for i := 0; i < len(request.Imp); i++ {
		var imp = request.Imp[i]
		var bidderExt ExtImpBidderYieldmo

		if err := json.Unmarshal(imp.Ext, &bidderExt); err != nil {
			return &errortypes.BadInput{
				Message: err.Error(),
			}
		}

		var yieldmoExt openrtb_ext.ExtImpYieldmo

		if err := json.Unmarshal(bidderExt.Bidder, &yieldmoExt); err != nil {
			return &errortypes.BadInput{
				Message: err.Error(),
			}
		}

		var impExt Ext
		impExt.PlacementId = yieldmoExt.PlacementId

		if bidderExt.Data != nil {
			if bidderExt.Data.PbAdslot != "" {
				impExt.Gpid = bidderExt.Data.PbAdslot
			}
		}

		impExtJSON, err := json.Marshal(impExt)
		if err != nil {
			return &errortypes.BadInput{
				Message: err.Error(),
			}
		}

		request.Imp[i].Ext = impExtJSON
	}

	return nil
}

// MakeBids make the bids for the bid response.
func (a *YieldmoAdapter) MakeBids(internalRequest *openrtb2.BidRequest, externalRequest *adapters.RequestData, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if response.StatusCode == http.StatusBadRequest {
		return nil, []error{&errortypes.BadInput{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode),
		}}
	}

	if response.StatusCode != http.StatusOK {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode),
		}}
	}

	var bidResp openrtb2.BidResponse

	if err := json.Unmarshal(response.Body, &bidResp); err != nil {
		return nil, []error{err}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(1)

	for _, sb := range bidResp.SeatBid {
		for i := range sb.Bid {
			bidType, err := getMediaTypeForImp(sb.Bid[i])
			if err != nil {
				continue
			}

			bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
				Bid:     &sb.Bid[i],
				BidType: bidType,
			})
		}
	}
	return bidResponse, nil
}

// Builder builds a new instance of the Yieldmo adapter for the given bidder with the given config.
func Builder(bidderName openrtb_ext.BidderName, config config.Adapter, server config.Server) (adapters.Bidder, error) {
	bidder := &YieldmoAdapter{
		endpoint: config.Endpoint,
	}
	return bidder, nil
}

// Retrieve the media type corresponding to the bid from the bid.ext object
func getMediaTypeForImp(bid openrtb2.Bid) (openrtb_ext.BidType, error) {
	var bidExt ExtBid
	if err := json.Unmarshal(bid.Ext, &bidExt); err != nil {
		return "", &errortypes.BadInput{Message: err.Error()}
	}

	switch bidExt.MediaType {
	case "banner":
		return openrtb_ext.BidTypeBanner, nil
	case "video":
		return openrtb_ext.BidTypeVideo, nil
	case "native":
		return openrtb_ext.BidTypeNative, nil
	default:
		return "", fmt.Errorf("invalid BidType: %s", bidExt.MediaType)
	}
}
