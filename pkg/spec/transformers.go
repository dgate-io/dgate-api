package spec

import (
	"encoding/base64"
	"encoding/json"
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/util/sliceutil"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TransformDGateRoutes(routes ...*DGateRoute) []*Route {
	rts := make([]*Route, len(routes))
	for i, r := range routes {
		rts[i] = TransformDGateRoute(r)
	}
	return rts
}

func TransformDGateRoute(r *DGateRoute) *Route {
	svcName := ""
	if r.Service != nil {
		svcName = r.Service.Name
	}
	if r.Namespace == nil {
		panic("route namespace is nil")
	}
	var modules []string
	if r.Modules != nil && len(r.Modules) > 0 {
		modules = sliceutil.SliceMapper(r.Modules, func(m *DGateModule) string { return m.Name })
	}
	return &Route{
		Name:          r.Name,
		Paths:         r.Paths,
		Methods:       r.Methods,
		PreserveHost:  r.PreserveHost,
		StripPath:     r.StripPath,
		ServiceName:   svcName,
		NamespaceName: r.Namespace.Name,
		Modules:       modules,
		Tags:          r.Tags,
	}
}

func TransformDGateModules(modules ...*DGateModule) []*Module {
	mods := make([]*Module, len(modules))
	for i, m := range modules {
		mods[i] = TransformDGateModule(m)
	}
	return mods
}

func TransformDGateModule(m *DGateModule) *Module {
	payload := ""
	if m.Payload != "" {
		payload = base64.StdEncoding.EncodeToString([]byte(m.Payload))
	}
	return &Module{
		Name:          m.Name,
		Payload:       payload,
		NamespaceName: m.Namespace.Name,
		Tags:          m.Tags,
	}
}

func TransformDGateServices(services ...*DGateService) []*Service {
	svcs := make([]*Service, len(services))
	for i, s := range services {
		svcs[i] = TransformDGateService(s)
	}
	return svcs
}

func TransformDGateService(s *DGateService) *Service {
	if s == nil {
		return nil
	}
	return &Service{
		Name:          s.Name,
		Tags:          s.Tags,
		NamespaceName: s.Namespace.Name,
		URLs: sliceutil.SliceMapper(s.URLs,
			func(u *url.URL) string { return u.String() }),
		Retries:        &s.Retries,
		HTTP2Only:      &s.HTTP2Only,
		RetryTimeout:   &s.RetryTimeout,
		TLSSkipVerify:  &s.TLSSkipVerify,
		ConnectTimeout: &s.ConnectTimeout,
		RequestTimeout: &s.RequestTimeout,
	}
}

func TransformDGateNamespaces(namespaces ...*DGateNamespace) []*Namespace {
	nss := make([]*Namespace, len(namespaces))
	for i, ns := range namespaces {
		nss[i] = TransformDGateNamespace(ns)
	}
	return nss
}

func TransformDGateNamespace(ns *DGateNamespace) *Namespace {
	return &Namespace{
		Name: ns.Name,
		Tags: ns.Tags,
	}
}
func TransformDGateDomains(domains ...*DGateDomain) []*Domain {
	doms := make([]*Domain, len(domains))
	for i, dom := range domains {
		doms[i] = TransformDGateDomain(dom)
	}
	return doms
}

func TransformDGateDomain(dom *DGateDomain) *Domain {
	return &Domain{
		Name:          dom.Name,
		NamespaceName: dom.Namespace.Name,
		Patterns:      dom.Patterns,
		// set as empty string to avoid sending cert/key to client
		Cert: dom.Cert,
		Key:  dom.Key,
		Tags: dom.Tags,
	}
}

func TransformDGateCollections(collections ...*DGateCollection) []*Collection {
	cols := make([]*Collection, len(collections))
	for i, col := range collections {
		cols[i] = TransformDGateCollection(col)
	}
	return cols
}

func TransformDGateCollection(col *DGateCollection) *Collection {
	var schema any
	if len(col.SchemaPayload) > 0 {
		err := json.Unmarshal([]byte(col.SchemaPayload), &schema)
		if err != nil {
			panic(err)
		}
	}
	return &Collection{
		Name:          col.Name,
		NamespaceName: col.Namespace.Name,
		Schema:        schema,
		// Type:          col.Type,
		Visibility: col.Visibility,
		Tags:       col.Tags,
	}
}

func TransformDGateDocuments(documents ...*DGateDocument) []*Document {
	docs := make([]*Document, len(documents))
	for i, doc := range documents {
		docs[i] = TransformDGateDocument(doc)
	}
	return docs
}

func TransformDGateDocument(document *DGateDocument) *Document {
	var payloadStruct any
	if document.Data != "" {
		err := json.Unmarshal([]byte(document.Data), &payloadStruct)
		if err != nil {
			panic(err)
		}
	}
	return &Document{
		ID:             document.ID,
		NamespaceName:  document.Namespace.Name,
		CollectionName: document.Collection.Name,
		Data:           payloadStruct,
	}
}

func TransformDGateSecrets(secrets ...*DGateSecret) []*Secret {
	newSecrets := make([]*Secret, len(secrets))
	for i, secret := range secrets {
		newSecrets[i] = TransformDGateSecret(secret)
	}
	return newSecrets
}

func TransformDGateSecret(sec *DGateSecret) *Secret {
	return &Secret{
		Name:          sec.Name,
		NamespaceName: sec.Namespace.Name,
		Data:          "**redacted**",
		Tags:          sec.Tags,
	}
}

func TransformRoutes(routes ...Route) []*DGateRoute {
	rts := make([]*DGateRoute, len(routes))
	for i, r := range routes {
		rts[i] = TransformRoute(r)
	}
	return rts
}

func TransformRoute(r Route) *DGateRoute {
	svc := &DGateService{}
	if r.ServiceName != "" {
		svc.Name = r.ServiceName
	}
	return &DGateRoute{
		Name:         r.Name,
		Paths:        r.Paths,
		Methods:      r.Methods,
		PreserveHost: r.PreserveHost,
		StripPath:    r.StripPath,
		Service:      svc,
		Namespace:    &DGateNamespace{Name: r.NamespaceName},
		Modules:      sliceutil.SliceMapper(r.Modules, func(m string) *DGateModule { return &DGateModule{Name: m} }),
		Tags:         r.Tags,
	}
}

func TransformModules(ns *DGateNamespace, modules ...*Module) ([]*DGateModule, error) {
	mods := make([]*DGateModule, len(modules))
	var err error
	for i, m := range modules {
		mods[i], err = TransformModule(ns, m)
		if err != nil {
			panic(err)
		}
	}
	return mods, nil
}

func TransformModule(ns *DGateNamespace, m *Module) (*DGateModule, error) {
	payload, err := base64.StdEncoding.DecodeString(m.Payload)
	if err != nil {
		return nil, err
	}
	return &DGateModule{
		Name:      m.Name,
		Namespace: ns,
		Payload:   string(payload),
		Tags:      m.Tags,
		Type:      m.Type,
	}, nil
}

func TransformServices(ns *DGateNamespace, services ...*Service) []*DGateService {
	svcs := make([]*DGateService, len(services))
	for i, s := range services {
		svcs[i] = TransformService(ns, s)
	}
	return svcs
}

func TransformService(ns *DGateNamespace, s *Service) *DGateService {
	return &DGateService{
		Name:      s.Name,
		Tags:      s.Tags,
		Namespace: ns,
		URLs: sliceutil.SliceMapper(s.URLs, func(u string) *url.URL {
			url, _ := url.Parse(u)
			return url
		}),
		Retries:        or(s.Retries, 3),
		HTTP2Only:      or(s.HTTP2Only, false),
		RetryTimeout:   or(s.RetryTimeout, 0),
		TLSSkipVerify:  or(s.TLSSkipVerify, false),
		ConnectTimeout: or(s.ConnectTimeout, 0),
		RequestTimeout: or(s.RequestTimeout, 0),
	}
}

func TransformNamespaces(namespaces ...*Namespace) []*DGateNamespace {
	nss := make([]*DGateNamespace, len(namespaces))
	for i, ns := range namespaces {
		nss[i] = TransformNamespace(ns)
	}
	return nss
}

func TransformNamespace(ns *Namespace) *DGateNamespace {
	return &DGateNamespace{
		Name: ns.Name,
		Tags: ns.Tags,
	}
}

func TransformDomains(ns *DGateNamespace, domains ...*Domain) []*DGateDomain {
	doms := make([]*DGateDomain, len(domains))
	for i, dom := range domains {
		doms[i] = TransformDomain(ns, dom)
	}
	return doms
}

func TransformDomain(ns *DGateNamespace, dom *Domain) *DGateDomain {
	return &DGateDomain{
		Name:      dom.Name,
		Namespace: ns,
		Patterns:  dom.Patterns,
		Cert:      dom.Cert,
		Key:       dom.Key,
		Tags:      dom.Tags,
	}
}

func TransformCollections(ns *DGateNamespace, mods []*DGateModule, collections ...*Collection) []*DGateCollection {
	cols := make([]*DGateCollection, len(collections))
	for i, col := range collections {
		cols[i] = TransformCollection(ns, mods, col)
	}
	return cols
}

func TransformCollection(ns *DGateNamespace, mods []*DGateModule, col *Collection) *DGateCollection {
	var schema *jsonschema.Schema
	var schemaData []byte
	if col.Schema != nil {
		var err error
		schemaData, err = json.Marshal(col.Schema)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(schemaData, &schema)
		if err != nil {
			panic(err)
		}
		schema = jsonschema.MustCompileString(
			col.Name+".json", string(schemaData))
	}
	return &DGateCollection{
		Name:          col.Name,
		Namespace:     ns,
		Schema:        schema,
		SchemaPayload: string(schemaData),
		// Type:          col.Type,
		// Modules:       mods,
		Visibility: col.Visibility,
		Tags:       col.Tags,
	}
}

func TransformDocuments(ns *DGateNamespace, col *DGateCollection, documents ...*Document) []*DGateDocument {
	docs := make([]*DGateDocument, len(documents))
	for i, doc := range documents {
		docs[i] = TransformDocument(ns, col, doc)
	}
	return docs
}

func TransformDocument(ns *DGateNamespace, col *DGateCollection, document *Document) *DGateDocument {
	var payload string
	if document.Data != nil {
		payloadBytes, err := json.Marshal(document.Data)
		if err != nil {
			panic(err)
		}
		payload = string(payloadBytes)
	}
	return &DGateDocument{
		ID:         document.ID,
		Namespace:  ns,
		Collection: col,
		Data:       payload,
	}
}

func or[T any](v *T, def T) T {
	if v == nil {
		return def
	}
	return *v
}

func TransformSecrets(ns *DGateNamespace, secrets ...*Secret) ([]*DGateSecret, error) {
	var err error
	scrts := make([]*DGateSecret, len(secrets))
	for i, secret := range secrets {
		scrts[i], err = TransformSecret(ns, secret)
		if err != nil {
			return nil, err
		}
	}
	return scrts, nil
}

func TransformSecret(ns *DGateNamespace, secret *Secret) (*DGateSecret, error) {
	var payload string = ""
	if secret.Data != "" {
		var err error
		plBytes, err := base64.RawStdEncoding.DecodeString(secret.Data)
		if err != nil {
			return nil, err
		}
		payload = string(plBytes)
	}
	return &DGateSecret{
		Name:      secret.Name,
		Namespace: ns,
		Data:      payload,
		Tags:      secret.Tags,
	}, nil
}
