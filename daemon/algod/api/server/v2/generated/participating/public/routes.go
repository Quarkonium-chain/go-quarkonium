// Package public provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/algorand/oapi-codegen DO NOT EDIT.
package public

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	. "github.com/Quarkonium-chain/go-quarkonium/daemon/algod/api/server/v2/generated/model"
	"github.com/algorand/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
)

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Get a list of unconfirmed transactions currently in the transaction pool by address.
	// (GET /v2/accounts/{address}/transactions/pending)
	GetPendingTransactionsByAddress(ctx echo.Context, address string, params GetPendingTransactionsByAddressParams) error
	// Broadcasts a raw transaction or transaction group to the network.
	// (POST /v2/transactions)
	RawTransaction(ctx echo.Context) error
	// Get a list of unconfirmed transactions currently in the transaction pool.
	// (GET /v2/transactions/pending)
	GetPendingTransactions(ctx echo.Context, params GetPendingTransactionsParams) error
	// Get a specific pending transaction.
	// (GET /v2/transactions/pending/{txid})
	PendingTransactionInformation(ctx echo.Context, txid string, params PendingTransactionInformationParams) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// GetPendingTransactionsByAddress converts echo context to params.
func (w *ServerInterfaceWrapper) GetPendingTransactionsByAddress(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameterWithLocation("simple", false, "address", runtime.ParamLocationPath, ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	ctx.Set(Api_keyScopes, []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params GetPendingTransactionsByAddressParams
	// ------------- Optional query parameter "max" -------------

	err = runtime.BindQueryParameter("form", true, false, "max", ctx.QueryParams(), &params.Max)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max: %s", err))
	}

	// ------------- Optional query parameter "format" -------------

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetPendingTransactionsByAddress(ctx, address, params)
	return err
}

// RawTransaction converts echo context to params.
func (w *ServerInterfaceWrapper) RawTransaction(ctx echo.Context) error {
	var err error

	ctx.Set(Api_keyScopes, []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.RawTransaction(ctx)
	return err
}

// GetPendingTransactions converts echo context to params.
func (w *ServerInterfaceWrapper) GetPendingTransactions(ctx echo.Context) error {
	var err error

	ctx.Set(Api_keyScopes, []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params GetPendingTransactionsParams
	// ------------- Optional query parameter "max" -------------

	err = runtime.BindQueryParameter("form", true, false, "max", ctx.QueryParams(), &params.Max)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter max: %s", err))
	}

	// ------------- Optional query parameter "format" -------------

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetPendingTransactions(ctx, params)
	return err
}

// PendingTransactionInformation converts echo context to params.
func (w *ServerInterfaceWrapper) PendingTransactionInformation(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "txid" -------------
	var txid string

	err = runtime.BindStyledParameterWithLocation("simple", false, "txid", runtime.ParamLocationPath, ctx.Param("txid"), &txid)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter txid: %s", err))
	}

	ctx.Set(Api_keyScopes, []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params PendingTransactionInformationParams
	// ------------- Optional query parameter "format" -------------

	err = runtime.BindQueryParameter("form", true, false, "format", ctx.QueryParams(), &params.Format)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter format: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.PendingTransactionInformation(ctx, txid, params)
	return err
}

// This is a simple interface which specifies echo.Route addition functions which
// are present on both echo.Echo and echo.Group, since we want to allow using
// either of them for path registration
type EchoRouter interface {
	CONNECT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	HEAD(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	OPTIONS(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	TRACE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router EchoRouter, si ServerInterface, m ...echo.MiddlewareFunc) {
	RegisterHandlersWithBaseURL(router, si, "", m...)
}

// Registers handlers, and prepends BaseURL to the paths, so that the paths
// can be served under a prefix.
func RegisterHandlersWithBaseURL(router EchoRouter, si ServerInterface, baseURL string, m ...echo.MiddlewareFunc) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET(baseURL+"/v2/accounts/:address/transactions/pending", wrapper.GetPendingTransactionsByAddress, m...)
	router.POST(baseURL+"/v2/transactions", wrapper.RawTransaction, m...)
	router.GET(baseURL+"/v2/transactions/pending", wrapper.GetPendingTransactions, m...)
	router.GET(baseURL+"/v2/transactions/pending/:txid", wrapper.PendingTransactionInformation, m...)

}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"H4sIAAAAAAAC/+x9/XPctpLgv4Kb3Srb2qEkfyT74qvUnmInedrYsctSsvvW8iUYsmcGTyTAB4DzEZ//",
	"9ys0ABIkwRmOpNjxbn6yNSSBRqPR6O9+P0lFUQoOXKvJ0/eTkkpagAaJf9E0FRXXCcvMXxmoVLJSM8En",
	"T/0zorRkfDGZTpj5taR6OZlOOC2gecd8P51I+EfFJGSTp1pWMJ2odAkFNQPrbWnerkfaJAuRuCHO7BDn",
	"zycfdjygWSZBqT6Ur3i+JYyneZUB0ZJyRVPzSJE100uil0wR9zFhnAgORMyJXrZeJnMGeaaO/SL/UYHc",
	"Bqt0kw8v6UMDYiJFDn04n4lixjh4qKAGqt4QogXJYI4vLakmZgYDq39RC6KAynRJ5kLuAdUCEcILvCom",
	"T99OFPAMJO5WCmyF/51LgN8g0VQuQE/eTWOLm2uQiWZFZGnnDvsSVJVrRfBdXOOCrYAT89UxeVkpTWZA",
	"KCdvvntGHj9+/JVZSEG1hswR2eCqmtnDNdnPJ08nGdXgH/dpjeYLISnPkvr9N989w/kv3ALHvkWVgvhh",
	"OTNPyPnzoQX4DyMkxLiGBe5Di/rNF5FD0fw8g7mQMHJP7Mt3uinh/J90V1Kq02UpGNeRfSH4lNjHUR4W",
	"fL6Lh9UAtN4vDaakGfTtafLVu/cPpw9PP/zT27Pkv9yfXzz+MHL5z+px92Ag+mJaSQk83SYLCRRPy5Ly",
	"Pj7eOHpQS1HlGVnSFW4+LZDVu2+J+dayzhXNK0MnLJXiLF8IRagjowzmtMo18ROTiueGTZnRHLUTpkgp",
	"xYplkE0N910vWbokKVV2CHyPrFmeGxqsFGRDtBZf3Y7D9CFEiYHrRvjABf1xkdGsaw8mYIPcIElzoSDR",
	"Ys/15G8cyjMSXijNXaUOu6zI5RIITm4e2MsWcccNTef5lmjc14xQRSjxV9OUsDnZioqscXNydo3fu9UY",
	"rBXEIA03p3WPmsM7hL4eMiLImwmRA+WIPH/u+ijjc7aoJCiyXoJeujtPgioFV0DE7O+QarPt/37x6kci",
	"JHkJStEFvKbpNQGeigyyY3I+J1zogDQcLSEOzZdD63BwxS75vythaKJQi5Km1/EbPWcFi6zqJd2woioI",
	"r4oZSLOl/grRgkjQleRDANkR95BiQTf9SS9lxVPc/2balixnqI2pMqdbRFhBN1+fTh04itA8JyXwjPEF",
	"0Rs+KMeZufeDl0hR8WyEmKPNngYXqyohZXMGGalH2QGJm2YfPIwfBk8jfAXg+EEGwaln2QMOh02EZszp",
	"Nk9ISRcQkMwx+ckxN3yqxTXwmtDJbIuPSgkrJipVfzQAI069WwLnQkNSSpizCI1dOHQYBmPfcRy4cDJQ",
	"KrimjENmmDMCLTRYZjUIUzDhbn2nf4vPqIIvnwzd8c3Tkbs/F91d37njo3YbX0rskYxcneapO7Bxyar1",
	"/Qj9MJxbsUVif+5tJFtcmttmznK8if5u9s+joVLIBFqI8HeTYgtOdSXh6RU/Mn+RhFxoyjMqM/NLYX96",
	"WeWaXbCF+Sm3P70QC5ZesMUAMmtYowoXflbYf8x4cXasN1G94oUQ11UZLihtKa6zLTl/PrTJdsxDCfOs",
	"1nZDxeNy45WRQ7/Qm3ojB4AcxF1JzYvXsJVgoKXpHP/ZzJGe6Fz+Zv4py9x8rct5DLWGjt2VjOYDZ1Y4",
	"K8ucpdQg8Y17bJ4aJgBWkaDNGyd4oT59H4BYSlGC1MwOSssyyUVK80RpqnGkf5Ywnzyd/NNJY385sZ+r",
	"k2DyF+arC/zIiKxWDEpoWR4wxmsj+qgdzMIwaHyEbMKyPRSaGLebaEiJGRacw4pyfdyoLC1+UB/gt26m",
	"Bt9W2rH47qhggwgn9sUZKCsB2xfvKRKgniBaCaIVBdJFLmb1D/fPyrLBID4/K0uLD5QegaFgBhumtHqA",
	"y6fNSQrnOX9+TL4Px0ZRXPB8ay4HK2qYu2Hubi13i9W2JbeGZsR7iuB2CnlstsajwYj5d0FxqFYsRW6k",
	"nr20Yl7+q3s3JDPz+6iPPw8SC3E7TFyoaDnMWR0HfwmUm/sdyukTjjP3HJOz7rc3Ixszyg6CUecNFu+a",
	"ePAXpqFQeykhgCigJrc9VEq6nTghMUFhr08mPymwFFLSBeMI7dSoT5wU9Nruh0C8G0IAVetFlpasBFmb",
	"UJ3M6VB/3LOzfAbUGttYL4kaSTVnSqNejS+TJeQoOFPuCToklRtRxogN37GIGua1pKWlZffEil2Moz5v",
	"X7Kw3vLiHXknRmEO2H2w0QjVjdnyXtYZhQS5RgeGb3KRXv+VquUdnPCZH6tP+zgNWQLNQJIlVcvIwenQ",
	"djPaGPo2LyLNklkw1XG9xBdioe5gibk4hHWV5TOa52bqPsvqrBYHHnWQ85yYlwkUDA3mTnG0Fnarf5Fv",
	"abo0YgFJaZ5PG1ORKJMcVpAbpZ1xDnJK9JLq5vDjyF6vwXOkwDA7DSRYjTMzoYlN1rYICaSgeAMVRpsp",
	"8/Y3NQdVtICOFIQ3oqjQihAoGufP/epgBRx5Uj00gl+vEa014eDHZm73CGfmwi7OWgC1d9/V+Kv5RQto",
	"83Zzn/JmCiEza7PW5jcmSSqkHcLe8G5y8x+gsvnYUuf9UkLihpB0BVLR3Kyus6gHNfne1encczIzqmlw",
	"Mh0VxhUwyznwOxTvQEasNK/wPzQn5rGRYgwlNdTDUBgRgTs1sxezQZWdybyA9lZBCmvKJCVNrw+C8lkz",
	"eZzNjDp531rrqdtCt4h6hy43LFN3tU042NBetU+ItV15dtSTRXYynWCuMQi4FCWx7KMDguUUOJpFiNjc",
	"+bX2jdjEYPpGbHpXmtjAneyEGWc0s/9GbJ47yITcj3kcewzSzQI5LUDh7cZDxmlmafxyZzMhbyZNdC4Y",
	"ThpvI6Fm1ECYmnaQhK9WZeLOZsRjYV/oDNQEeOwWArrDxzDWwsKFpr8DFpQZ9S6w0B7orrEgipLlcAek",
	"v4wKcTOq4PEjcvHXsy8ePvrl0RdfGpIspVhIWpDZVoMi951Zjii9zeFBVDtC6SI++pdPvI+qPW5sHCUq",
	"mUJBy/5Q1vdltV/7GjHv9bHWRjOuugZwFEcEc7VZtBPr1jWgPYdZtbgArY2m+1qK+Z1zw94MMejwpdel",
	"NIKFavsJnbR0kplXTmCjJT0p8U3gmY0zMOtgyuiAxexOiGpo47Nmlow4jGaw91Acuk3NNNtwq+RWVndh",
	"3gAphYxewaUUWqQiT4ycx0TEQPHavUHcG367yu7vFlqypoqYudF7WfFswA6hN3z8/WWHvtzwBjc7bzC7",
	"3sjq3Lxj9qWN/EYLKUEmesMJUmfLPDKXoiCUZPghyhrfg7byFyvgQtOifDWf3421U+BAETsOK0CZmYh9",
	"w0g/ClLBbTDfHpONG3UMerqI8V4mPQyAw8jFlqfoKruLYztszSoYR7+92vI0MG0ZGHPIFi2yvL0Jawgd",
	"dqp7KgKOQccLfIy2+ueQa/qdkJeN+Pq9FFV55+y5O+fY5VC3GOcNyMy33gzM+CJvB5AuDOzHsTV+kgU9",
	"q40Idg0IPVLkC7ZY6kBffC3F73AnRmeJAYoPrLEoN9/0TUY/iswwE12pOxAlm8EaDmfoNuRrdCYqTSjh",
	"IgPc/ErFhcyBkEOMdcIQLR3KrWifYIrMwFBXSiuz2qokGIDUuy+aDxOa2hOaIGrUQPhFHTdj37LT2XC2",
	"XALNtmQGwImYuRgHF32Bi6QYPaW9mOZE3Ai/aMFVSpGCUpAlzhS9FzT/nr069A48IeAIcD0LUYLMqbw1",
	"sNervXBewzbBWD9F7v/ws3rwCeDVQtN8D2LxnRh6u/a0PtTjpt9FcN3JQ7KzljpLtUa8NQwiBw1DKDwI",
	"J4P714Wot4u3R8sKJIaU/K4U7ye5HQHVoP7O9H5baKtyIILdqelGwjMbxikXXrCKDZZTpZN9bNm81LIl",
	"mBUEnDDGiXHgAcHrBVXahkExnqFN014nOI8VwswUwwAPqiFm5J+9BtIfOzX3IFeVqtURVZWlkBqy2BrQ",
	"Izs414+wqecS82DsWufRglQK9o08hKVgfIcspwHjH1TX/lfn0e0vDn3q5p7fRlHZAqJBxC5ALvxbAXbD",
	"KN4BQJhqEG0Jh6kO5dShw9OJ0qIsDbfQScXr74bQdGHfPtM/Ne/2ics6Oey9nQlQ6EBx7zvI1xazNn57",
	"SRVxcHgXO5pzbLxWH2ZzGBPFeArJLspHFc+8FR6BvYe0KheSZpBkkNNtJDjAPib28a4BcMcbdVdoSGwg",
	"bnzTG0r2cY87hhY4nooJjwSfkNQcQaMKNATivt4zcgY4dow5OTq6Vw+Fc0W3yI+Hy7ZbHRkRb8OV0GbH",
	"HT0gyI6jjwF4AA/10DdHBX6cNLpnd4q/gXIT1HLE4ZNsQQ0toRn/oAUM2IJdjlNwXjrsvcOBo2xzkI3t",
	"4SNDR3bAMP2aSs1SVqKu8wNs71z1604QdZyTDDRlOWQkeGDVwDL8ntgQ0u6YN1MFR9ne+uD3jG+R5fgw",
	"nTbw17BFnfu1zU0ITB13octGRjX3E+UEAfURz0YED1+BDU11vjWCml7ClqxBAlHVzIYw9P0pWpRJOEDU",
	"P7NjRuedjfpGd7qLL3CoYHmxWDOrE+yG77KjGLTQ4XSBUoh8hIWsh4woBKNiR0gpzK4zl/7kE2A8JbWA",
	"dEwbXfP19X9PtdCMKyB/ExVJKUeVq9JQyzRCoqCAAqSZwYhg9ZwuOLHBEORQgNUk8cnRUXfhR0duz5ki",
	"c1j7nEHzYhcdR0dox3ktlG4drjuwh5rjdh65PtBxZS4+p4V0ecr+iCc38pidfN0ZvPZ2mTOllCNcs/xb",
	"M4DOydyMWXtII+OivXDcUb6cdnxQb9247xesqHKq78JrBSuaJ2IFUrIM9nJyNzET/NsVzV/Vn2E+JKSG",
	"RlNIUsziGzkWXJpvbOKfGYdxZg6wDfofCxCc268u7Ed7VMwmUpUVBWSMasi3pJSQgs13M5Kjqpd6TGwk",
	"fLqkfIEKgxTVwgW32nGQ4VfKmmZkxXtDRIUqveEJGrljF4ALU/Mpj0acAmpUuq6F3Cowa1rP57Jcx9zM",
	"wR50PQZRJ9l0MqjxGqSuGo3XIqedtzniMmjJewF+molHulIQdUb26eMr3BZzmMzm/j4m+2boGJT9iYOI",
	"3+bhUNCvUbfz7R0IPXYgIqGUoPCKCs1Uyj4V8zBH24cKbpWGom/Jt5/+MnD83gzqi4LnjENSCA7baFkS",
	"xuElPoweJ7wmBz5GgWXo264O0oK/A1Z7njHUeFv84m53T2jXY6W+E/KuXKJ2wNHi/QgP5F53u5vypn5S",
	"mucR16LL4OwyADWtg3WZJFQpkTKU2c4zNXVRwdYb6dI92+h/Xeel3MHZ647b8aGFxQHQRgx5SShJc4YW",
	"ZMGVllWqrzhFG1Ww1EgQl1fGh62Wz/wrcTNpxIrphrriFAP4astVNGBjDhEzzXcA3nipqsUClO7oOnOA",
	"K+7eYpxUnGmcqzDHJbHnpQSJkVTH9s2Cbsnc0IQW5DeQgswq3Zb+MUFZaZbnzqFnpiFifsWpJjlQpclL",
	"xi83OJx3+vsjy0GvhbyusRC/3RfAQTGVxIPNvrdPMa7fLX/pYvwx3N0+9kGnTcWEiVlmq0jK/73/b0/f",
	"niX/RZPfTpOv/uXk3fsnHx4c9X589OHrr/9f+6fHH75+8G//HNspD3ssfdZBfv7cacbnz1H9CUL1u7B/",
	"NPt/wXgSJbIwmqNDW+Q+lopwBPSgbRzTS7jiesMNIa1ozjLDW25CDt0bpncW7enoUE1rIzrGML/WA5WK",
	"W3AZEmEyHdZ4YymqH58ZT1RHp6TLPcfzMq+43Uovfds8TB9fJubTuhiBrVP2lGCm+pL6IE/356MvvpxM",
	"mwzz+vlkOnFP30UomWWbWB2BDDYxXTFMkrinSEm3CnSceyDs0VA6G9sRDltAMQOplqz8+JxCaTaLczif",
	"suRsTht+zm2Avzk/6OLcOs+JmH98uLUEyKDUy1j9opaghm81uwnQCTsppVgBnxJ2DMddm09m9EUX1JcD",
	"nfvAVCnEGG2oPgeW0DxVBFgPFzLKsBKjn056g7v81Z2rQ27gGFzdOWMRvfe+//aSnDiGqe7ZkhZ26KAI",
	"QUSVdsmTrYAkw83CnLIrfsWfwxytD4I/veIZ1fRkRhVL1UmlQH5Dc8pTOF4I8tTnYz6nml7xnqQ1WFgx",
	"SJomZTXLWUquQ4WkIU9bLKs/wtXVW5ovxNXVu15sRl99cFNF+YudIDGCsKh04kr9JBLWVMZ8X6ou9YIj",
	"21peu2a1QraorIHUlxJy48d5Hi1L1S350F9+WeZm+QEZKlfQwGwZUVrU+WhGQHEpvWZ/fxTuYpB07e0q",
	"lQJFfi1o+ZZx/Y4kV9Xp6WPM7GtqIPzqrnxDk9sSRltXBktSdI0quHCrVmKselLSRczFdnX1VgMtcfdR",
	"Xi7QxpHnBD9rZR36BAMcqllAneI8uAEWjoOTg3FxF/YrX9YxvgR8hFvYTsC+1X4F+fM33q49Ofi00svE",
	"nO3oqpQhcb8zdbW3hRGyfDSGYgvUVl1hvBmQdAnptatYBkWpt9PW5z7gxwmannUwZWvZ2QxDrKaEDooZ",
	"kKrMqBPFKd92y9oom1GBg76Ba9heiqYY0yF1bNplVdTQQUVKDaRLQ6zhsXVjdDffRZX5RFNXnQSTNz1Z",
	"PK3pwn8zfJCtyHsHhzhGFK2yH0OIoDKCCEv8Ayi4wULNeLci/djyGE+Ba7aCBHK2YLNYGd7/6PvDPKyG",
	"Kl3lQReFXA+oCJsTo8rP7MXq1HtJ+QLM9WyuVKFobquqRoM2UB9aApV6BlTvtPPzsCCFhw5VyjVmXqOF",
	"b2qWABuz30yjxY7D2mgVaCiy77jo5ePh+DMLOGQ3hMd/3mgKx4O6rkNdpOKgv5Vr7NZqrQvNC+kM4bLP",
	"C8CSpWJt9sVAIVy1TVvUJbhfKkUXMKC7hN67kfUwWh4/HGSfRBKVQcS8K2r0JIEoyPblxKw5eobBPDGH",
	"GNXMTkCmn8k6iJ3PCItoO4TNchRg68hVu/dUtryotirwEGhx1gKSN6KgB6ONkfA4LqnyxxHrpXouO0o6",
	"+x3LvuwqTXcexBIGRVHrwnP+Nuxy0J7e7wrU+ap0vhRdqPSPKCtndC9MX4hth+AommaQw8Iu3L7sCaUp",
	"mNRskIHj1XyOvCWJhSUGBupAAHBzgNFcjgixvhEyeoQYGQdgY+ADDkx+FOHZ5ItDgOSu4BP1Y+MVEfwN",
	"8cQ+G6hvhFFRmsuVDfgbU88BXCmKRrLoRFTjMITxKTFsbkVzw+acLt4M0quQhgpFpx6aC715MKRo7HBN",
	"2Sv/oDVZIeEmqwmlWQ90XNTeAfFMbBKboRzVRWabmaH3aO4C5kvHDqatRXdPkZnYYDgXXi02Vn4PLMNw",
	"eDAC28uGKaRX/G5IzrLA7Jp2t5wbo0KFJOMMrTW5DAl6Y6YekC2HyOV+UF7uRgB0zFBNrwZnlthrPmiL",
	"J/3LvLnVpk3ZVJ8WFjv+Q0couksD+Ovbx9oF4f7aFP4bLi7mT9RHqYTXtyzdpkKh/bi0VQcPKVDYJYcW",
	"EDuw+rorB0bR2o71auM1wFqMlRjm23dK9tGmIAdUgpOWaJpcxyIFjC4PeI9f+M8CYx3uHuXbB0EAoYQF",
	"Uxoap5GPC/oU5niK5ZOFmA+vTpdybtb3Roj68rduc/ywtcyPvgKMwJ8zqXSCHrfoEsxL3yk0In1nXo1L",
	"oO0QRdtsgGVxjovTXsM2yVhexenVzfvDczPtj/VFo6oZ3mKM2wCtGTbHiAYu75jaxrbvXPALu+AX9M7W",
	"O+40mFfNxNKQS3uOz+RcdBjYLnYQIcAYcfR3bRClOxhkkHDe546BNBrEtBzv8jb0DlPmx94bpebT3odu",
	"fjtSdC1BGcB4hqBYLCDz5c28P4wHReRywRdBF6ey3FUz75jY0nVYeW5H0ToXhg9DQfiBuJ8wnsEmDn2o",
	"FSDkTWYdFtzDSRbAbbmSuFkoipowxB/fCGx1H9kX2k0AiAZBX3ac2U10st2lejtxA3KgmdNJFPj17T6W",
	"/Q1xqJsOhU+3Kp/uPkI4INIU00Fjk34ZggEGTMuSZZuO48mOOmgEowdZlwekLWQtbrA9GGgHQUcJrlVK",
	"24VaOwP7Ceq8J0Yrs7HXLrDY0DdNXQJ+Vkn0YLQim/t122tdbeTaf/j5QgtJF+C8UIkF6VZD4HIOQUNQ",
	"FV0RzWw4Scbmcwi9L+omnoMWcD0bezaCdCNEFnfRVIzrL5/EyGgP9TQw7kdZnGIitDDkk7/se7m8TB+Y",
	"kuorIdiaG7iqoun6P8A2+ZnmlVEymFRNeK5zO7Uv3wN2fVX8AFsceW/UqwFsz66g5ekNIA3GLP31IxUU",
	"sL6nWiX+Ub1sbeEBO3UW36U72hrXlGGY+JtbptW0oL2U2xyMJkjCwDJmNy7isQnm9EAb8V1S3rcJLNsv",
	"gwTyfjgVU76FZf8qqmtR7KPdS6C5J15czuTDdHK7SIDYbeZG3IPr1/UFGsUzRppaz3ArsOdAlNOylGJF",
	"88TFSwxd/lKs3OWPr/vwio+sycQp+/LbsxevHfgfppM0ByqT2hIwuCp8r/xsVmXbOOy+Smy1b2fotJai",
	"YPPrisxhjMUaK3t3jE29pihN/ExwFF3MxTwe8L6X97lQH7vEHSE/UNYRP43P0wb8tIN86Iqy3DsbPbQD",
	"wem4uHGddaJcIRzg1sFCQcxXcqfspne646ejoa49PAnneoWlKeMaB3eFK5EVueAfeufS03dCtpi/y0yM",
	"Bg/9fmKVEbItHgditX3/yq4wdUys4PXr4ldzGo+OwqN2dDQlv+buQQAg/j5zv6N+cXQU9R5GzViGSaCV",
	"itMCHtRZFoMb8XEVcA7rcRf02aqoJUsxTIY1hdooII/utcPeWjKHz8z9kkEO5qfjMUp6uOkW3SEwY07Q",
	"xVAmYh1kWtiWmYoI3o2pxiRYQ1rI7F1LBuuM7R8hXhXowExUztJ4aAefKcNeuQ2mNC8TfHnAWmtGrNhA",
	"bC6vWDCWeW1MzdQOkMEcUWSqaNnWBncz4Y53xdk/KiAsM1rNnIHEe61z1XnlAEftCaRxu5gb2PqpmuFv",
	"YwfZ4W/ytqBdRpCd/rvntU/JLzTW9OfACPBwxh7j3hG97ejDUbPNZlu2QzDH6TFjWqd7RuecdQNzRFuh",
	"M5XMpfgN4o4Q9B9FCmF4xydDM+9vwGORe12WUjuVm47uzez7tnu8bjy08bfWhf2i665jN7lM46f6sI28",
	"idKr4uWaHZKHlLAwwqCdGjDAWvB4BcGw2AbFRx9Rbs+TrQLRyjCLn8owl/PEjt+cSgdzL/81p+sZjfWI",
	"MbqQgSnY3laclBbEf+w3QNU1DuzsJIjgrt9ltpJcCbLxQfSr0t5Qr7HTjtZoGgUGKSpUXaY2TCFXIjJM",
	"xdeU2y7i5jvLr9zXCqwL3ny1FhLrQKp4SFcGKSui5tirq7dZ2g/fydiC2QbZlYKgA7MbiNhik0hFrot1",
	"XbnDoeZ8Tk6nQRt4txsZWzHFZjngGw/tGzOq8Lqs3eH1J2Z5wPVS4euPRry+rHgmIdNLZRGrBKl1TxTy",
	"6sDEGeg1ACen+N7Dr8h9DMlUbAUPDBadEDR5+vArDKixf5zGblnX4HwXy86QZ/tg7TgdY0yqHcMwSTdq",
	"PPp6LgF+g+HbYcdpsp+OOUv4prtQ9p+lgnK6gHh+RrEHJvst7ia68zt44dYbAEpLsSVMx+cHTQ1/Gsj5",
	"NuzPgkFSURRMFy5wT4nC0FPTXtlO6oezvf5dvygPl3+I8a+lD//r2Lo+shpDi4GcLYxS/hF9tCFap4Ta",
	"4p85ayLTfb9Ocu5rC2MDrbpvlsWNmcssHWVJDFSfk1IyrtH+Uel58hejFkuaGvZ3PARuMvvySaQRVbtX",
	"Cz8M8I+OdwkK5CqOejlA9l5mcd+S+1zwpDAcJXvQ1FgITuVgoG48JHMoLnT30GMlXzNKMkhuVYvcaMCp",
	"b0V4fMeAtyTFej0H0ePBK/volFnJOHnQyuzQT29eOCmjEDLWMKA57k7ikKAlgxVmzMU3yYx5y72Q+ahd",
	"uA30nzb+yYucgVjmz3JUEQg8mruS5Y0U//PLpvI5OlZtJmLHBihkxNrp7HYfOdrwMKtb139rA8bw2QDm",
	"RqMNR+ljZSD63obX1998inihLkh2z1sGx4e/Eml0cJTjj44Q6KOjqRODf33UfmzZ+9FRvABx1ORmfm2w",
	"cBuNGL+N7eE3ImIA810L64AiVx8hYoAcuqTMA8MEZ26oKWl3iPv4UsTd5HfFo03jp+Dq6i0+8XjAP7qI",
	"+MTMEjewyVIYPuztDplRksnq50GcOyXfiM1YwuncQZ54/gAoGkDJSPMcrqTXATTqrt8bLxLQqBl1Brkw",
	"SmbYFCi0538+eDaLn+7AdsXy7OemtlvnIpGUp8tolPDMfPiLldFbV7BlldE+I0vKOeTR4axu+4vXgSNa",
	"+t/F2HkKxke+2+1Aa5fbWVwDeBtMD5Sf0KCX6dxMEGK1XTarLsuQL0RGcJ6mqUXDHPutnGMtNCP5zThs",
	"UWkXt4q54K7g0JzlGIYZ9xvjm4mkeqCAFvY79/2FzDjYflxZM4MdHSShrMCLWdGizAFP5gokXeCngkPn",
	"cyyhhiMHHSuIKs0jfBMLVgiiK8mJmM+DZQDXTEK+nZKSKmUHOTXLgg3OPXn68PQ0avZC7IxYqcWiX+ar",
	"ZikPT/AV+8Q1WbKtAA4Cdj+sHxqKOmRj+4Tjekr+owKlYzwVH9jMVfSSmlvb9pOse58ek++x8pEh4lap",
	"ezRX+iLC7YKaVZkLmk2xuPHlt2cviJ3VfmNbyNt+lgu01rXJP+peGV9g1Fd2GqicM36c3aU8zKqVTur2",
	"k7HahOaNpkEm68TcoB0vxM4xeW5NqHUDfzsJwRLZsoAs6HZplXgkDvMfrWm6RNtkSwIa5pXjG7F6dtZ4",
	"boLsw7r7ETJsA7frxWpbsU6J0EuQa6YAM/JhBe1yiHVtUGcb9+UR28uTFeeWUo4PEEbrXkeHot0DZyVZ",
	"H1QQhayD+AMtU7Yf86F9aS/wq3guRqfJbcfr74vr+RLb5KVzLqSUC85SbIUQk6SxdNs4N+WIrhFx/6Ka",
	"uBMaOVzR1rp1LrDD4mCzXc8IHeL6Lv/gqdlUSx32Tw0b13JtAVo5zgbZ1He6dg4xxhW4blaGiEI+KWQk",
	"qCmaCFEHUBxIRliVacDC+Z159qOzf2NRjGvG0dLl0Ob0M+uyyhVDzzQnTJOFAOXW087mUW/NN8dYpTGD",
	"zbvjF2LB0gu2wDFsGJ1Zto0Z7Q915iNIXcSmefeZedfVzq9/boWD2UnPytJNOtwHPSpI6g0fRHAsbskH",
	"kgTIrccPR9tBbjtDv/E+NYQGK4xagxLv4R5h1L2026N8a3RLS1H4BrEZldECuoxHwHjBuHehxi+INHol",
	"4MbgeR34TqWSaqs7jOJpl0DzgQQIzFC2PvjbDtXtHGBQgmv0cwxvY9MGfIBx1C80Ej/lW+IPhaHuQJh4",
	"RvM6dDrS1BulKidEZZhc1GnzHWMchnEnPmWyha696Xv159iN49CbaKhG4azKFqATmmWx0lbf4FOCT32S",
	"GGwgreomVHV2YLtGeZ/a3ESp4KoqdszlX7jldEHf/Ag1hL37/Q5jpZ3ZFv+NdWAa3hkXNH1wVq6PkM4O",
	"K8zfzzKOSb2GphPFFsl4TOCdcnt0NFPfjNCb7++U0n267h8iG7fD5cI9ivG3b83FERbu7cWn26ulrquL",
	"seACn/uCR3VFyDZXwqus12cMox5w8yJb1gHevxgFfEXzgUz40Fdi71frPxjKh08HyzdQ7cpzaUp2sqDB",
	"kkc2Vrjjfem7EIfig2148N15LdxadyJ02Hf3Q8tTZ2PEGmYx6KG7mROt2eBDvWg/rIZKJPg+Hfg87Afi",
	"onimrgw8rJiofPSVj4H2KqH91ZXgafX9GFh/NLPgU3stBn0sl65/rV2m08l/+Nl6YQlwLbd/AI9Lb9O7",
	"TWUi0q41TzWvkLr14ahWiK1bcUwPm1i7FCcbeluZZS0tWuq1n+mR1fMx4kAPHx+mk/PsoAsz1nJnYkeJ",
	"HbsXbLHUWLH/r0AzkK/3dCRouhDgESuFYk0H0twM5krALnG447HJBoaAWdhRoT+WD0JdQaqx7WwTXCcB",
	"DumvYCbzTp8/OxMMq9N1ToZrSLCrC0G/1+yeO75XOCko/mX7dB6Pr7l/VodQ2wywNVVNuZZOzvTozM35",
	"HFKsiryzUNV/LIEHRZCm3i6DsMyDulWszmPCut6HWx0bgHbVkdoJT9Bf59bgDOWxX8P2niItaog2Dq2T",
	"+G5SOBgxYF1gvob0kCHZRY0xVVMGYsGHBLtSzE1zjMGaz0HZtRvO5UnSXBxNKbYdU8abno+ay3x6UNlH",
	"TMkZqmXV75k8rH88xxbVygXI0brwcKilk/N+45y1K1yMZcVq34kvYQzK/+ZrCNpZcnbt+gcgVqynak1l",
	"5t+4k6JQ9m5icaDn9cysSeDoBzlEWjFgLlSaCyNGJEMJZe2ciTrg8J6ykaFNAR+Eaw5SQla7RHKhINHC",
	"J3zsgmMXKmz4642QoAbbH1ngBktfv2lqe2MbOIqlrqmLeg0XSCQU1EAngwrcw3PuQvYz+9wn4fs2YHst",
	"TDW97u9H61N3mOohMaT6OXG35f7k/psYmxjnIBPveeqW4+btimxYdzOrUntBhwejNsiNrp2zg5VE7TRp",
	"f5UdHSFIkr+G7YlVgnwjX7+DIdBWcrKgBwVHO5t8p+Y3FYN7cSfgfdo6cqUQeTLg7Djv1xDvUvw1S68B",
	"awDWIe4DPdrJfbSx197s9XLra2aXJXDIHhwTcsZtUpF3bLfbC3Ym5/f0rvk3OGtW2bL+zqh2fMXj2RlY",
	"cF/ekpv5YXbzMAWG1d1yKjvIngrVGz4UcrPG4vztLp7HY7Xyvqu520W+ISoLRUwmubAeq2d40GOGIyyB",
	"ENTqQEcmJc7TRVQuYrG8NynTYIaKYyqcDAHSwMdUC6ihcINHERDtix45hbb0nSt6J+ZEQuNEvmn1v34L",
	"95hG3525nqXN7+ZCQqsZu/naVvqsE1+wjCb+Z8a0pHJ7kxp9vRbyPevJIJb3hmPVkVjNQpporD4O81ys",
	"E2RWSd3nIqbamvdU+zL2Tdea78ypnkEQ10WVE9S2ZEkzkgopIQ2/iOd7WqgKISHJBYZ5xTzQc23k7gKT",
	"vDjJxYKIMhUZ2H4xcQoamqvinKLYBEFUTRQFlnYwW9h+E9DxyCnNnWr9SAmKWosDeuenYDPXm6pOdtGJ",
	"9WUORCyDclWcHIbsy314d/T+j/PmOdsg3YCMHfk50bKCKXFvdHtku4NPJZCCKWVBqWlpzfIcE8fZJvC8",
	"1oELcdQOiL3nGFa5Yhh70y4iYKXh0tx5dWWFkAdchGWPiF5KUS2WQYHpGk6v8srKKcThKD+pCsOjMIPM",
	"TPGEFEJpp2nakZolNyFn91PBtRR53jZKWRF94SztL+nmLE31CyGuZzS9foB6LRe6Xmk29fnV3eDAZibZ",
	"KS3WvoAT2858f6le+x6GyjmiHc0gOyzu4MbuAZjv9nPQ/Tb3s/7CuutqM9O4GnPGCdWiYGn8TH1e0XaD",
	"MXIxFhWtWWZ7K9oqE/gaHvbwsqqDK5BF9tEMnEabw50RxwickxnZjfkvSuDdcckcHKMZuCj7zMVJUUk6",
	"KOt1AEBIbeqzrqRtyBhKYjVXEQtbKgFd5F1AR94qGIl0O9jMCHcOlIZbAdWLfqwBvG+ND1NbW85GUs7E",
	"xj9/0BSfuxHwH3ZTeYt5DIV4XTSkJW2Qly9UM8AR4iWud8ZDXWLa+2xsVFTdPHfkDR8AMBwn1YJhVLTU",
	"oWDMKcshS2K9F89rG9U00LRdala3JTpTjpOntPKtD83YlQRXOMWK+LLt/yqpISVRv963JPMMNmDzOn4D",
	"KWxPw2ngf4HctjzsGANEmeSwglb4mKvmUqGoyVbgv1X1xyQDKNEb2bWRxeKiwru8Yzhxa0+CyJox2I1a",
	"Uixi7U6RPWaSqFFnwxN7TNTYo2QgWrGsoi38qUNFjrYZ0BzlCKp6OkLi9cix0/xkR3jjBzjz38dEGY+J",
	"d+P40MEsKI66XQxob5xkpYZOPY+HSYalimoHC86W1Y5YS+IN31AlXfNhg2Sf5Bt1a+Q+McEDxH67gRSl",
	"GqfvQOY0ngEnhat6gtTOATKrFZhPItb2JXDCRdBick1Vrao0NRT9D3ZifIlxp03fwKncRDPefmcJDkZU",
	"p5jaoCIhazq9uXn+k5zEnQdxcLwYjShw6X877F+eup3agS9gK29u9tPI/tik0d1ijotPyazyA+W5WNue",
	"kaEe+hy8H9RSn3cBObGc1deyj9qcuvKeXVMHC+LVC7olQuI/Ruv8R0VzNt8in7Hg+8+IWlJDQs7xaiMC",
	"XBSomXi3eDX1gHlri/BT2XWzsWMGw23NKAHQ5iL3zX0EKeg1hNuAwQ6Wf6baME5VzdByYa7sznb2seAW",
	"70u0FDQLNX0sFNluo+5LB5uv/3eTCxdO5eu7lTlNfYdQ16KozWewC7AnLr2EYneyZJ+veRKoOws3RCt9",
	"dn12A5PpgawrloEw1H6lBXav42qv88ytljHS8tvpsbEjzXTUUu56F8ZG3fSADvs07gM/bFv5cfAfreE6",
	"tIwx4P9R8D7QqDaE1/ak/QhYblXgiMBqrdUzsUkkzNW+ABNrrjbqvGxqd3gTK+OpBKpsxM35K6d4NiVK",
	"GTeKsI0JrX2a9SgZzBlvmCXjZaUjegxWKuXbAGGh0R/ROuBCG5ISjDC5ovmrFUjJsqGNM6fDtnQMW0R4",
	"R4f7NmLCqO/U/gBMNToc5mc2ZvTwNXOB2yZUNlxTacozKrPwdcZJCtLc+2RNt+rmHqXaObDPp0QDaaZd",
	"NSDwLiFpW0DyrXMK39LfUwNI79DxM8Jhg3HBEWeNNe1oMeCf6cPwWThsCrpJcrHALMKBA+Fq06KHz6qA",
	"gqMZ3Mpn49bt51HsN9g9DZbld4xIC5x1zBS7z/0r3EpUI3/iTO88+dZG2U3rtHG39mB6pPJFE/xviaV/",
	"HmOZuK74SpiN64VNn6riaQ+CTYQB/1DbLj6wixgG4dK4QyP4+HZn7UiLWL6vtQwkaDFQO8L7QTWh7DR1",
	"4Vl9U1rP1GCRMnXZ0gda2qx93t9LA+DZ3vTurLenrUNmzDiH9IjbnR+dlKJM0jExn7ZzR+bcBA7SNowD",
	"9BE4AQbWXYfHqLqXTavuUaupzaFt8gab6uzzdpXpLqV/yEw0wNHbLggxR15mO7ejdQszeWpjyrSbY9Y2",
	"g9VMglAiIa0kmonXdLu/7dhAxeiLv5598fDRL4+++JKYF0jGFqCaquOdtl1NXCDjXbvPx40E7C1PxzfB",
	"Vx+wiPP+R59UVW+KO2uW26qmpGivadkh9uXIBRA5jpF2UTfaKxynCe3/Y21XbJF3vmMxFPz+eyZFnse7",
	"PtRyVcSBEtutwIViNJASpGJKG0bY9oAy3UREqyWaB7H278pWkxE8BW8/dlTA9EDIVWwhQwG1yM8wt9t5",
	"jQhsytzxKuvp2bUup6dZCx0KjRgVMwNSitKJ9mxOYhBhBpEMMmud4RMt4kGMbM1sbbRsjBBd5Hmc9MKG",
	"2bu5fbuZq45zerOJEfHCH8obkOaQf2K4bsFNOElj2v/D8I9IIYY74xr1cn8PXhHVD27WlH8UaP2k/Ah5",
	"IAAD2batPMkgUSwoRCytlwD9Cd6B3BU/XjaO5b1pIQiJ/2APeGH6bPNencngwPnEFX1f1kgJlvJuiBJa",
	"y9+XketZb32RBFvkjCZag7JsSfTFwiDdWj2rs5gHtJJesrMUQhOjmeZ5JEna2nHwTIWEY1QCuaL5x+ca",
	"3zGp9BniA7I3w6lRYaZsiGSLSnWzOn0v6Ki5g6zYu5uav8bE7P8As0fRe84N5ZzwvdsMjTvYsX7hbwWb",
	"603WOKYNsnr4JZm5ZhulhJSprnN/7YWTOjEUJJu7gFbY6D2ZqPvW+bPQtyDjuY/EIT8G7q3aZ+8gbI7o",
	"J2YqAyc3SuUx6uuRRQR/MR4VNufdc13csjHDzcq+BAXcDiz70m87PHZ5trSJuXQqBf11jr6tW7iNXNTN",
	"2sbWLBrd3+Hq6q2ejSk1FO/FYD7HWkd30pThoJYMv0OVI4sjN4abN0YxPw/VvbW1XQdqc3f2o2L53oCV",
	"VqX1D9PJAjgoprCW+C+ud8zHvUs9BLbyQv+oWlhvUy7GIiay1tbkwVRBDfUR5dPdZ5Ga15jVmFaS6S32",
	"DfYGNPZLtB7T93VtD1cbpvalubtPi2uoe7c3lUAq5W/X7wXN8T6yLj5ubiGRH5NvbYVvd1C+vjf7V3j8",
	"lyfZ6eOH/zr7y+kXpyk8+eKr01P61RP68KvHD+HRX754cgoP519+NXuUPXryaPbk0ZMvv/gqffzk4ezJ",
	"l1/96z3DhwzIFlBf2v/p5D+Ts3whkrPX58mlAbbBCS3ZD2D2BnXlucC+lgapKZ5EKCjLJ0/9T//Hn7Dj",
	"VBTN8P7XievPNFlqXaqnJyfr9fo4/ORkgan/iRZVujzx82C3wZa88vq8jtG3cTi4o431GDfVkcIZPnvz",
	"7cUlOXt9ftwQzOTp5PT49Piha23NackmTyeP8Sc8PUvc9xOsr3miXOn8kzpX68O096wsbWF988jRqPtr",
	"CTTHAjvmjwK0ZKl/JIFmW/d/taaLBchjzN6wP60enXhp5OS9q5zwYdezkzAy5OR9q8BEtudLH/mw75WT",
	"97517u4BW21TXcyZQWrU5fk9aFduydoeIrU60NPgRp8ShXXzzU+lZMKc16m5fDPAuAAMb5NYQFzLiqfW",
	"WWynAI7/fXn2n+gwf3n2n+Rrcjp1CQcKFZrY9Dbjuia088yC3Y9TVN9sz+pqJo1zffL0bczI5IJFy2qW",
	"s5RYOQUPqqHC4BzVIzZ8Ei2KE1X3N2+4vuHkp8lX795/8ZcPMWmyJxvXSAoKfLS8vsJ3PkWkFXTz9RDK",
	"Ni4C3Yz7jwrktllEQTeTEOC+BzVS9cwnCPkG0GFsYhC1+O8Xr34kQhKnPb+m6XWdHOWz4ZoMwDAZznw5",
	"BLG7WEOggVeFuaNcllWhFmW7AHCN5nfYLREBRXby6PTU81CnoQQH9MSd+2CmjlmrT2gYphMYKvup8IrA",
	"hqY63xKqgjgJjFr0nU07KWyiTFqB9DtNo/0Z3ZZEsxAOzcaPVKgXmuZ74LvsdIFsocOF/JTmkt2f/t5D",
	"RhSCdzExItxaTyN/7u5/j93tSyWkFOZMM4zLbq4cf521gHSyaL714A4UGjkmfxMVyo5GK6g0xHrg4wzW",
	"J+LmdHWRgkC6JnUInxwddRd+dNSE/c1hjUyWcnyxi46jo2OzU08OZGU77dStMsKjzs4hw/U26yXd1FHT",
	"lHDBEw4LqtkKSKBwPjl9+Nmu8JzbOHUjLFuh/sN08sVnvGXn3Ag2NCf4pl3N4892NRcgVywFcglFKSSV",
	"LN+Sn3idCBC0WO+zv5/4NRdr7hFh9NWqKKjcOiGa1jyn4kHfn538p1fhqBG0kYvShcJYGBRRrUzrqyDy",
	"xeTdB68DjNQ9dr12MsMOmGNfhVBhGdZO0DOhTt6jbX3w9xPnII0/RB+HVZ5PfO3FgTdtla34w5ZW9F5v",
	"zEJ2D2feCcZLqU6XVXnyHv+DenCwIlu0/0Rv+AnGhJ68byHCPe4hov1783n4xqoQGXjgxHyuUI/b9fjk",
	"vf03mAg2JUhmriMslOl+tQWNT7CT9Lb/85an0R/762gVcx34+cSbYWIqdfvN960/2zSllpXOxDqYBR0Y",
	"1vvWh8w8rFT375M1ZdoISa6GKJ1rkP2PNdD8xDUM6vza1OjvPcHGA8GPHbGqFLaIUFujfUPXl61cUGmL",
	"ZXwj0FAxxHA3yYxx5EIhl2zMkvZhX0Xq8cbLJdj4W+/ZjcigWpCZFDRLqdLmD9daq6cbf7il/tWt7XEe",
	"8dshmGhu6JejNPzkeK8zB8cdI2QG+0LOn/sJmwS0310w60H0Dc2IrzqVkJc0NxsOGTlz4n8LG7+3UPXp",
	"paBPLLZ8NDnjG3/4FKFYgq+lIMp40ZygB94YocJokYYBLIAnjgUlM5FtXZuyiaRrvbE1OrrM7YS2b4y2",
	"IZJKWqihh3dgpfxjmyb3WST/NAT+aQj801T0pyHwz9390xA40hD4p5nsTzPZ/0gz2SG2sZiY6cw/w9Im",
	"9k2nrXmt3keb/hQ1i29XD2O6lslaaaTYCoPpY0IusfQLNbcErEDSnKRUWenKlSkqMLoTa5BB9vSKJy1I",
	"bAylmfh+818bvHpVnZ4+BnL6oPuN0izPQ97c/xblXXxk80u+JleTq0lvJAmFWEFmk2HD+uj2q73D/q96",
	"3Fe9xgqYBY+1dXypMqKq+ZylzKI8F3xB6EI0gddYkJULfALSAGfbUxGmpy5RhbnsaNe9vl3GvS259yWA",
	"82YL94YUdMglHk1gCO/AUIJ/GRNH8D9aSr9pNavbMtKdY/e46p9c5WNwlU/OVz53J21gWvxvKWY+OX3y",
	"2S4oNET/KDT5DpMKbieOuUKhabRL100FLV8oxpv7msDkMNAXb9E6xPftO3MRKJArf8E2catPT06wcthS",
	"KH0yMddfO6Y1fPiuhvm9v51KyVbYBhqtm0KyBeM0T1zgZ9LEpj46Pp18+P8BAAD//3e/DWz5IQEA",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %s", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	var res = make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	var resolvePath = PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		var pathToFile = url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
