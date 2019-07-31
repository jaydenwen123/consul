package api

import (
	"bytes"
	"fmt"
	"io"
	"time"
)

// Intention defines an intention for the Connect Service Graph. This defines
// the allowed or denied behavior of a connection between two services using
// Connect.
type Intention struct {
	// ID is the UUID-based ID for the intention, always generated by Consul.
	ID string

	// Description is a human-friendly description of this intention.
	// It is opaque to Consul and is only stored and transferred in API
	// requests.
	Description string

	// SourceNS, SourceName are the namespace and name, respectively, of
	// the source service. Either of these may be the wildcard "*", but only
	// the full value can be a wildcard. Partial wildcards are not allowed.
	// The source may also be a non-Consul service, as specified by SourceType.
	//
	// DestinationNS, DestinationName is the same, but for the destination
	// service. The same rules apply. The destination is always a Consul
	// service.
	SourceNS, SourceName           string
	DestinationNS, DestinationName string

	// SourceType is the type of the value for the source.
	SourceType IntentionSourceType

	// Action is whether this is a whitelist or blacklist intention.
	Action IntentionAction

	// DefaultAddr, DefaultPort of the local listening proxy (if any) to
	// make this connection.
	DefaultAddr string
	DefaultPort int

	// Meta is arbitrary metadata associated with the intention. This is
	// opaque to Consul but is served in API responses.
	Meta map[string]string

	// Precedence is the order that the intention will be applied, with
	// larger numbers being applied first. This is a read-only field, on
	// any intention update it is updated.
	Precedence int

	// CreatedAt and UpdatedAt keep track of when this record was created
	// or modified.
	CreatedAt, UpdatedAt time.Time

	CreateIndex uint64
	ModifyIndex uint64
}

// String returns human-friendly output describing ths intention.
func (i *Intention) String() string {
	return fmt.Sprintf("%s => %s (%s)",
		i.SourceString(),
		i.DestinationString(),
		i.Action)
}

// SourceString returns the namespace/name format for the source, or
// just "name" if the namespace is the default namespace.
func (i *Intention) SourceString() string {
	return i.partString(i.SourceNS, i.SourceName)
}

// DestinationString returns the namespace/name format for the source, or
// just "name" if the namespace is the default namespace.
func (i *Intention) DestinationString() string {
	return i.partString(i.DestinationNS, i.DestinationName)
}

func (i *Intention) partString(ns, n string) string {
	// For now we omit the default namespace from the output. In the future
	// we might want to look at this and show this in a multi-namespace world.
	if ns != "" && ns != IntentionDefaultNamespace {
		n = ns + "/" + n
	}

	return n
}

// IntentionDefaultNamespace is the default namespace value.
const IntentionDefaultNamespace = "default"

// IntentionAction is the action that the intention represents. This
// can be "allow" or "deny" to whitelist or blacklist intentions.
type IntentionAction string

const (
	IntentionActionAllow IntentionAction = "allow"
	IntentionActionDeny  IntentionAction = "deny"
)

// IntentionSourceType is the type of the source within an intention.
type IntentionSourceType string

const (
	// IntentionSourceConsul is a service within the Consul catalog.
	IntentionSourceConsul IntentionSourceType = "consul"

	// IntentionSourceExternalTrustDomain is a service whose identity is not provided
	// by Connect and will present a SPIFFE ID whose trust domain (i.e. hostname)
	// is the same as that set in the intention's SourceName.
	IntentionSourceExternalTrustDomain IntentionSourceType = "ext-trust-domain"

	// IntentionSourceExternalURI is a service whose identity is not provided
	// by Connect and will present a SPIFFE ID equal to the ID set in the
	// intention's SourceName.
	IntentionSourceExternalURI IntentionSourceType = "ext-uri"
)

// IntentionMatch are the arguments for the intention match API.
type IntentionMatch struct {
	By    IntentionMatchType
	Names []string
}

// IntentionMatchType is the target for a match request. For example,
// matching by source will look for all intentions that match the given
// source value.
type IntentionMatchType string

const (
	IntentionMatchSource      IntentionMatchType = "source"
	IntentionMatchDestination IntentionMatchType = "destination"
)

// IntentionCheck are the arguments for the intention check API. For
// more documentation see the IntentionCheck function.
type IntentionCheck struct {
	// Source and Destination are the source and destination values to
	// check. The destination is always a Consul service, but the source
	// may be other values as defined by the SourceType.
	Source, Destination string

	// SourceType is the type of the value for the source.
	SourceType IntentionSourceType
}

// Intentions returns the list of intentions.
func (h *Connect) Intentions(q *QueryOptions) ([]*Intention, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/connect/intentions")
	r.setQueryOptions(q)
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out []*Intention
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return out, qm, nil
}

// IntentionGet retrieves a single intention.
func (h *Connect) IntentionGet(id string, q *QueryOptions) (*Intention, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/connect/intentions/"+id)
	r.setQueryOptions(q)
	rtt, resp, err := h.c.doRequest(r)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	if resp.StatusCode == 404 {
		return nil, qm, nil
	} else if resp.StatusCode != 200 {
		var buf bytes.Buffer
		io.Copy(&buf, resp.Body)
		return nil, nil, fmt.Errorf(
			"Unexpected response %d: %s", resp.StatusCode, buf.String())
	}

	var out Intention
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return &out, qm, nil
}

// IntentionDelete deletes a single intention.
func (h *Connect) IntentionDelete(id string, q *WriteOptions) (*WriteMeta, error) {
	r := h.c.newRequest("DELETE", "/v1/connect/intentions/"+id)
	r.setWriteOptions(q)
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	qm := &WriteMeta{}
	qm.RequestTime = rtt

	return qm, nil
}

// IntentionMatch returns the list of intentions that match a given source
// or destination. The returned intentions are ordered by precedence where
// result[0] is the highest precedence (if that matches, then that rule overrides
// all other rules).
//
// Matching can be done for multiple names at the same time. The resulting
// map is keyed by the given names. Casing is preserved.
func (h *Connect) IntentionMatch(args *IntentionMatch, q *QueryOptions) (map[string][]*Intention, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/connect/intentions/match")
	r.setQueryOptions(q)
	r.params.Set("by", string(args.By))
	for _, name := range args.Names {
		r.params.Add("name", name)
	}
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out map[string][]*Intention
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return out, qm, nil
}

// IntentionCheck returns whether a given source/destination would be allowed
// or not given the current set of intentions and the configuration of Consul.
func (h *Connect) IntentionCheck(args *IntentionCheck, q *QueryOptions) (bool, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/connect/intentions/check")
	r.setQueryOptions(q)
	r.params.Set("source", args.Source)
	r.params.Set("destination", args.Destination)
	if args.SourceType != "" {
		r.params.Set("source-type", string(args.SourceType))
	}
	rtt, resp, err := requireOK(h.c.doRequest(r))
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out struct{ Allowed bool }
	if err := decodeBody(resp, &out); err != nil {
		return false, nil, err
	}
	return out.Allowed, qm, nil
}

// IntentionCreate will create a new intention. The ID in the given
// structure must be empty and a generate ID will be returned on
// success.
func (c *Connect) IntentionCreate(ixn *Intention, q *WriteOptions) (string, *WriteMeta, error) {
	r := c.c.newRequest("POST", "/v1/connect/intentions")
	r.setWriteOptions(q)
	r.obj = ixn
	rtt, resp, err := requireOK(c.c.doRequest(r))
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{}
	wm.RequestTime = rtt

	var out struct{ ID string }
	if err := decodeBody(resp, &out); err != nil {
		return "", nil, err
	}
	return out.ID, wm, nil
}

// IntentionUpdate will update an existing intention. The ID in the given
// structure must be non-empty.
func (c *Connect) IntentionUpdate(ixn *Intention, q *WriteOptions) (*WriteMeta, error) {
	r := c.c.newRequest("PUT", "/v1/connect/intentions/"+ixn.ID)
	r.setWriteOptions(q)
	r.obj = ixn
	rtt, resp, err := requireOK(c.c.doRequest(r))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	wm := &WriteMeta{}
	wm.RequestTime = rtt
	return wm, nil
}
