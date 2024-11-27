package config

import (
	"errors"

	"github.com/dgate-io/dgate-api/pkg/spec"
)

func (resources *DGateResources) Validate() (int, error) {
	var numChanges int
	if resources == nil || resources.SkipValidation {
		return 0, nil
	}
	namespaces := make(map[string]*spec.Namespace)
	services := make(map[string]*spec.Service)
	routes := make(map[string]*spec.Route)
	domains := make(map[string]*spec.Domain)
	modules := make(map[string]*spec.Module)
	collections := make(map[string]*spec.Collection)
	documents := make(map[string]*spec.Document)
	secrets := make(map[string]*spec.Secret)

	for _, ns := range resources.Namespaces {
		if _, ok := namespaces[ns.Name]; ok {
			return 0, errors.New("duplicate namespace: " + ns.Name)
		}
		if ns.Name == "" {
			return 0, errors.New("namespace name must be specified")
		}
		namespaces[ns.Name] = &ns
	}
	numChanges += len(namespaces)

	for _, mod := range resources.Modules {
		key := mod.Name + "-" + mod.NamespaceName
		if _, ok := modules[key]; ok {
			return 0, errors.New("duplicate module: " + mod.Name)
		}
		if mod.Name == "" {
			return 0, errors.New("module name must be specified")
		}
		if mod.NamespaceName != "" {
			if _, ok := namespaces[mod.NamespaceName]; !ok {
				return 0, errors.New("module (" + mod.Name + ") references non-existent namespace (" + mod.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("module (" + mod.Name + ") must specify namespace")
		}
		if mod.Payload == "" && mod.PayloadFile == "" {
			return 0, errors.New("module payload or payload file must be specified")
		}
		if mod.Payload != "" && mod.PayloadFile != "" {
			return 0, errors.New("module payload and payload file cannot both be specified")
		}
		modules[key] = &mod.Module
	}
	numChanges += len(modules)

	for _, svc := range resources.Services {
		key := svc.Name + "-" + svc.NamespaceName
		if _, ok := services[key]; ok {
			return 0, errors.New("duplicate service: " + svc.Name)
		}
		if svc.Name == "" {
			return 0, errors.New("service name must be specified")
		}
		if svc.NamespaceName != "" {
			if _, ok := namespaces[svc.NamespaceName]; !ok {
				return 0, errors.New("service (" + svc.Name + ") references non-existent namespace (" + svc.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("service (" + svc.Name + ") must specify namespace")
		}
		services[key] = &svc
	}
	numChanges += len(services)

	for _, route := range resources.Routes {
		key := route.Name + "-" + route.NamespaceName
		if _, ok := routes[key]; ok {
			return 0, errors.New("duplicate route: " + route.Name)
		}
		if route.Name == "" {
			return 0, errors.New("route name must be specified")
		}
		if route.ServiceName != "" {
			if _, ok := services[route.ServiceName+"-"+route.NamespaceName]; !ok {
				return 0, errors.New("route (" + route.Name + ") references non-existent service (" + route.ServiceName + ")")
			}
		}
		if route.NamespaceName != "" {
			if _, ok := namespaces[route.NamespaceName]; !ok {
				return 0, errors.New("route (" + route.Name + ") references non-existent namespace (" + route.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("route (" + route.Name + ") must specify namespace")
		}
		for _, modName := range route.Modules {
			if _, ok := modules[modName+"-"+route.NamespaceName]; !ok {
				return 0, errors.New("route (" + route.Name + ") references non-existent module (" + modName + ")")
			}
		}
		routes[key] = &route
	}
	numChanges += len(routes)

	for _, dom := range resources.Domains {
		key := dom.Name + "-" + dom.NamespaceName
		if _, ok := domains[key]; ok {
			return 0, errors.New("duplicate domain: " + dom.Name)
		}
		if dom.Name == "" {
			return 0, errors.New("domain name must be specified")
		}
		if dom.NamespaceName != "" {
			if _, ok := namespaces[dom.NamespaceName]; !ok {
				return 0, errors.New("domain (" + dom.Name + ") references non-existent namespace (" + dom.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("domain (" + dom.Name + ") must specify namespace")
		}
		if dom.Cert != "" && dom.CertFile != "" {
			return 0, errors.New("domain cert and cert file cannot both be specified")
		}
		if dom.Key != "" && dom.KeyFile != "" {
			return 0, errors.New("domain key and key file cannot both be specified")
		}
		if (dom.Cert == "") != (dom.Key == "") {
			return 0, errors.New("domain cert (file) and key (file) must both be specified, or neither")
		}
		domains[key] = &dom.Domain
	}
	numChanges += len(domains)

	for _, col := range resources.Collections {
		key := col.Name + "-" + col.NamespaceName
		if _, ok := collections[key]; ok {
			return 0, errors.New("duplicate collection: " + col.Name)
		}
		if col.Name == "" {
			return 0, errors.New("collection name must be specified")
		}
		if col.NamespaceName != "" {
			if _, ok := namespaces[col.NamespaceName]; !ok {
				return 0, errors.New("collection (" + col.Name + ") references non-existent namespace (" + col.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("collection (" + col.Name + ") must specify namespace")
		}
		if col.Schema == nil {
			return 0, errors.New("collection (" + col.Name + ") must specify schema")
		}
		if col.Visibility != spec.CollectionVisibilityPublic && col.Visibility != spec.CollectionVisibilityPrivate {
			return 0, errors.New("collection (" + col.Name + ") must specify visibility")
		}
		if col.Type != spec.CollectionTypeDocument && col.Type != spec.CollectionTypeFetcher {
			return 0, errors.New("collection (" + col.Name + ") must specify type")
		}
		// TODO: Uncomment when modules are supported for collections
		// for _, modName := range col.Modules {
		// 	if _, ok := modules[modName+"-"+col.NamespaceName]; !ok {
		// 		return 0, errors.New("collection (" + col.Name + ") references non-existent module (" + modName + ")")
		// 	}
		// }
		collections[key] = &col
	}
	numChanges += len(collections)

	for _, doc := range resources.Documents {
		key := doc.ID + "-" + doc.NamespaceName
		if _, ok := documents[key]; ok {
			return 0, errors.New("duplicate document: " + doc.ID)
		}
		if doc.ID == "" {
			return 0, errors.New("document ID must be specified")
		}
		if doc.NamespaceName != "" {
			if _, ok := namespaces[doc.NamespaceName]; !ok {
				return 0, errors.New("document (" + doc.ID + ") references non-existent namespace (" + doc.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("document (" + doc.ID + ") must specify namespace")
		}
		if doc.CollectionName != "" {
			if _, ok := collections[doc.CollectionName+"-"+doc.NamespaceName]; !ok {
				return 0, errors.New("document (" + doc.ID + ") references non-existent collection (" + doc.CollectionName + ")")
			}
		}
		documents[key] = &doc
	}
	numChanges += len(documents)

	for _, sec := range resources.Secrets {
		key := sec.Name + "-" + sec.NamespaceName
		if _, ok := secrets[key]; ok {
			return 0, errors.New("duplicate secret: " + sec.Name)
		}
		if sec.Name == "" {
			return 0, errors.New("secret name must be specified")
		}
		if sec.NamespaceName != "" {
			if _, ok := namespaces[sec.NamespaceName]; !ok {
				return 0, errors.New("secret (" + sec.Name + ") references non-existent namespace (" + sec.NamespaceName + ")")
			}
		} else {
			return 0, errors.New("secret (" + sec.Name + ") must specify namespace")
		}
		secrets[key] = &sec
	}
	numChanges += len(secrets)

	return numChanges, nil
}
