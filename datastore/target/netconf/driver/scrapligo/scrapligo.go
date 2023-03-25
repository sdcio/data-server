package scrapligo

import (
	"fmt"

	"github.com/beevik/etree"
	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/target/netconf/types"
	scraplinetconf "github.com/scrapli/scrapligo/driver/netconf"
	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/util"
)

type ScrapligoNetconfTarget struct {
	driver *scraplinetconf.Driver
}

// NewScrapligoNetconfTarget inits a new ScrapligoNetconfTarget which is already connected to the target node
func NewScrapligoNetconfTarget(cfg *config.SBI) (*ScrapligoNetconfTarget, error) {
	var opts []util.Option

	if cfg.Credentials != nil {
		opts = append(opts,
			options.WithAuthUsername(cfg.Credentials.Username),
			options.WithAuthPassword(cfg.Credentials.Password),
			options.WithTransportType("standard"),
			options.WithPort(cfg.Port),
		)
	}

	opts = append(opts,
		options.WithAuthNoStrictKey(),
		options.WithNetconfForceSelfClosingTags(),
	)

	// init the netconf driver
	d, err := scraplinetconf.NewDriver(cfg.Address, opts...)
	if err != nil {
		return nil, err
	}

	err = d.Open()
	if err != nil {
		return nil, err
	}

	return &ScrapligoNetconfTarget{
		driver: d,
	}, nil
}

func (snt *ScrapligoNetconfTarget) Close() error {
	return snt.driver.Close()
}

// EditConfig transforms the generalized EditConfig into the scrapligo implementation
func (snt *ScrapligoNetconfTarget) EditConfig(target string, config string) (*types.NetconfResponse, error) {
	// add the <config/> tag to the provided config data
	xdoc := fmt.Sprintf("<config>%s</config>", config)

	// send the edit config rpc
	resp, err := snt.driver.EditConfig("candidate", xdoc)
	if err != nil {
		return nil, err
	}
	if resp.Failed != nil {
		return nil, resp.Failed
	}

	// creating a new etree Document and parsing the netconf rpc result
	x := etree.NewDocument()
	err = x.ReadFromString(resp.Result)
	if err != nil {
		return nil, err
	}

	// return the rpc result
	return types.NewNetconfResponse(x), nil
}

func (snt *ScrapligoNetconfTarget) GetConfig(source string, filter string) (*types.NetconfResponse, error) {
	// prepare the filter to hand it to scrapli
	filterDoc := createFilterOption(filter)

	// execute the GetConfig rpc
	resp, err := snt.driver.GetConfig(source, filterDoc, options.WithNetconfForceSelfClosingTags())
	if err != nil {
		return nil, err
	}
	if resp.Failed != nil {
		return nil, resp.Failed
	}

	// creating a new etree Document and parsing the netconf rpc result
	x := etree.NewDocument()
	err = x.ReadFromString(resp.Result)
	if err != nil {
		return nil, err
	}

	// the actual config is contained under /rpc-reply/data/ in the result document.
	// so we are extracting that portion
	newRootXpath := "/rpc-reply/data/*"
	r := x.FindElement(newRootXpath)
	if r == nil {
		return nil, fmt.Errorf("unable to find %q in %s", newRootXpath, resp.Result)
	}

	// making the new retrieved path the new root element of the xml doc
	x.SetRoot(r)

	// return the rpc result
	return types.NewNetconfResponse(x), nil
}

func (snt *ScrapligoNetconfTarget) Commit() error {
	// execute the Commit rpc
	resp, err := snt.driver.Commit()
	if err != nil {
		return err
	}
	if resp.Failed != nil {
		return resp.Failed
	}
	return nil
}

func (snt *ScrapligoNetconfTarget) Discard() error {
	resp, err := snt.driver.Discard()
	if err != nil {
		return err
	}
	if resp.Failed != nil {
		return resp.Failed
	}
	return nil
}

// createFilterOption is a helper function that populates the Filter field for the internal Scrapligo RPC instantiation
func createFilterOption(filter string) util.Option {
	return func(x interface{}) error {
		oo, ok := x.(*scraplinetconf.OperationOptions)

		if !ok {
			return util.ErrIgnoredOption
		}
		oo.Filter = filter
		return nil
	}
}
