// Package watcher provides a Watcher which can be used to observe extra resources at runtime.
package watcher

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/oliveagle/jsonpath"
	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type WatchedResource struct {
	ApiVersion string
	Kind       string
	Name       string
	Namespace  string
}

type WatchedResourceWithPath struct {
	WatchedResource
	Path string
}

// pathData contains last known value of property and a list of Namespaced Names (SpecialResourceModules)
// that should be reconciled if that `Data` changes
type pathData struct {
	Data            []string
	NamespacedNames []types.NamespacedName
}

type Path = string

func SRMWFromWatchedResourceWithPath(wrp WatchedResourceWithPath) srov1beta1.SpecialResourceModuleWatch {
	return srov1beta1.SpecialResourceModuleWatch{
		ApiVersion: wrp.ApiVersion,
		Kind:       wrp.Kind,
		Name:       wrp.Name,
		Namespace:  wrp.Namespace,
		Path:       wrp.Path,
	}
}

func watchedResourceToUnstructured(wr WatchedResource) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(wr.ApiVersion)
	u.SetKind(wr.Kind)
	return u
}

func getAPIVersion(gvk schema.GroupVersionKind) string {
	if gvk.Group != "" {
		return fmt.Sprintf("%s/%s", gvk.Group, gvk.Version)
	} else {
		return gvk.Version
	}
}

type Watcher interface {
	// ReconcileWatches is a general function that updates list of watched resources based on given SpecialResourceModule
	ReconcileWatches(srov1beta1.SpecialResourceModule) error
}

func New(ctrl controller.Controller) Watcher {
	return &watcher{
		log:               zap.New(zap.UseDevMode(true)).WithName(utils.Print("watcher", utils.Blue)),
		ctrl:              ctrl,
		watchedResToPaths: make(map[WatchedResource][]string),
		watchedResToData:  make(map[WatchedResourceWithPath]pathData),
	}
}

type watcher struct {
	log  logr.Logger
	ctrl controller.Controller

	watchedResToPaths map[WatchedResource][]Path
	watchedResToData  map[WatchedResourceWithPath]pathData
}

func isEqualWatchedResource(watch1, watch2 srov1beta1.SpecialResourceModuleWatch) bool {
	if watch1.ApiVersion != watch2.ApiVersion ||
		watch1.Kind != watch2.Kind ||
		watch1.Name != watch2.Name ||
		watch1.Namespace != watch2.Namespace ||
		watch1.Path != watch2.Path {
		return false
	}
	return true
}

func checkIfContainsSRM(desiredWatches []srov1beta1.SpecialResourceModuleWatch, currentlyWatched srov1beta1.SpecialResourceModuleWatch) bool {
	for _, w := range desiredWatches {
		if isEqualWatchedResource(w, currentlyWatched) {
			return true
		}
	}
	return false
}

func (w *watcher) ReconcileWatches(srm srov1beta1.SpecialResourceModule) error {

	nn := types.NamespacedName{
		Name:      srm.GetName(),
		Namespace: srm.GetNamespace(),
	}

	// Removal of unneeded resources to be watched
	// Iterate over map of WatchedResourceWithPath to check if the resource is present in incoming SpecialResourceModule.
	// If it's absent - SRM's NamespaceName will be removed from the list of NamespacedNames to reconcile.
	for watchedResourceWithPath, pathData := range w.watchedResToData {
		removeFromNamespacedNamesToTrigger := !checkIfContainsSRM(srm.Spec.Watch, SRMWFromWatchedResourceWithPath(watchedResourceWithPath))

		if removeFromNamespacedNamesToTrigger {
			for idx, nnToTrigger := range pathData.NamespacedNames {
				if nnToTrigger == nn {
					w.log.Info("removing watched resource", "resource", watchedResourceWithPath, "namespacedName", nnToTrigger)
					pathData.NamespacedNames = append(pathData.NamespacedNames[:idx], pathData.NamespacedNames[idx+1:]...)
					w.watchedResToData[watchedResourceWithPath] = pathData
					break
				}
			}
		}

		// If there's no SRM to trigger reconciliation for for this resource+path: remove it.
		if len(pathData.NamespacedNames) == 0 {
			w.log.Info("empty list of CR to trigger for resource", "resource", watchedResourceWithPath)
			delete(w.watchedResToData, watchedResourceWithPath)

			if paths, ok := w.watchedResToPaths[watchedResourceWithPath.WatchedResource]; ok {
				for idx, path := range paths {
					if path == watchedResourceWithPath.Path {
						w.watchedResToPaths[watchedResourceWithPath.WatchedResource] = append(w.watchedResToPaths[watchedResourceWithPath.WatchedResource][:idx],
							w.watchedResToPaths[watchedResourceWithPath.WatchedResource][idx+1:]...)
						break
					}
				}

				if len(w.watchedResToPaths[watchedResourceWithPath.WatchedResource]) == 0 {
					w.log.Info("empty list of paths to observe", "resource", watchedResourceWithPath.WatchedResource)
					delete(w.watchedResToPaths, watchedResourceWithPath.WatchedResource)
				}
			}
		}
	}

	// Addition
	for _, toWatch := range srm.Spec.Watch {
		if err := w.tryAddResourceToWatch(toWatch, nn); err != nil {
			return err
		}
	}

	return nil
}

func (w *watcher) tryAddResourceToWatch(r srov1beta1.SpecialResourceModuleWatch, nnToTrigger types.NamespacedName) error {
	if w == nil {
		return errors.New("watcher is not initialized")
	}

	l := w.log.WithValues("resource", r, "triggers", nnToTrigger)

	wr := WatchedResource{
		ApiVersion: r.ApiVersion,
		Kind:       r.Kind,
		Name:       r.Name,
		Namespace:  r.Namespace,
	}

	if w.isAlreadyBeingWatched(wr, r.Path, nnToTrigger) {
		return nil
	}

	// Predicates are not utilized because client.Object inside a predicate func does not have enough context.
	// Multiple SRMs can depend on different paths of a single watched resource.
	// Inside predicate func, it's not known which SRM will be triggered and therefore which path to check.
	// Because of that reason, mapping function takes care of filtering out.
	if err := w.ctrl.Watch(
		&source.Kind{Type: watchedResourceToUnstructured(wr)},
		handler.EnqueueRequestsFromMapFunc(w.mapper)); err != nil {

		l.Error(err, "failed to start watching a resource")
		return err
	}

	// TODO: Potential race? Registering first, then adding to a map = mapper func might get invoked before adding to a map?
	w.addToWatched(wr, r.Path, nnToTrigger)
	l.Info("added resource to be watched")

	return nil
}

func (w *watcher) isAlreadyBeingWatched(wr WatchedResource, path string, nnToTrigger types.NamespacedName) bool {

	wrp := WatchedResourceWithPath{WatchedResource: wr, Path: path}
	if nns, ok := w.watchedResToData[wrp]; ok {
		for _, nn := range nns.NamespacedNames {
			if nn == nnToTrigger {
				return true
			}
		}
	}

	return false
}

func (w *watcher) addToWatched(wr WatchedResource, path string, nnToTrigger types.NamespacedName) {
	var ok bool
	var paths []Path
	addPath := true
	if paths, ok = w.watchedResToPaths[wr]; !ok {
		paths = make([]string, 0)
	} else {
		for _, p := range paths {
			if p == path {
				addPath = false
				break
			}
		}
	}
	if addPath {
		paths = append(paths, path)
	}
	w.watchedResToPaths[wr] = paths

	wrd := WatchedResourceWithPath{WatchedResource: wr, Path: path}
	var pd pathData
	if pd, ok = w.watchedResToData[wrd]; !ok {
		pd.NamespacedNames = make([]types.NamespacedName, 0)
	}
	pd.NamespacedNames = append(pd.NamespacedNames, nnToTrigger)
	w.watchedResToData[wrd] = pd
}

// mapper returns a list of NamespacedNames based on given Object.
//
// If Object's apiVersion+kind+name+namespace are registered to be watched,
// the paths in that object are inspected for changes.
// If the value in incoming object is different than stored value,
// a reconciliation will be triggered for SpecialResourceModules that depend on that object's value at path.
func (w *watcher) mapper(o client.Object) []reconcile.Request {
	gvk := o.GetObjectKind().GroupVersionKind()
	apiVer := getAPIVersion(gvk)

	wrObj := WatchedResource{
		ApiVersion: apiVer,
		Kind:       gvk.Kind,
	}

	crsToTrigger := []reconcile.Request{}

	var unstr *unstructured.Unstructured
	var ok bool
	if unstr, ok = o.(*unstructured.Unstructured); !ok {
		w.log.Info("failed to convert incoming object to Unstructed")
		return crsToTrigger
	}

	//TODO cache resources, maybe? take the resourceversion and generation?
	paths, ok := w.watchedResToPaths[wrObj]
	if !ok {
		wrObj.Name = o.GetName()
		wrObj.Namespace = o.GetNamespace()
		paths, ok = w.watchedResToPaths[wrObj]
	}
	if ok {
		for _, path := range paths {
			// Get current value of property at path
			vals, err := GetJSONPath(path, *unstr)
			if err != nil {
				w.log.Error(err, "failed to obtain a value", "path", path)
			}

			// Compare obtained value against stored one
			wrd := WatchedResourceWithPath{WatchedResource: wrObj, Path: path}
			if pathData, ok := w.watchedResToData[wrd]; ok {
				if !areEqual(pathData.Data, vals) {
					for _, nn := range pathData.NamespacedNames {
						crsToTrigger = append(crsToTrigger, reconcile.Request{NamespacedName: nn})
					}

					pathData.Data = vals
					w.watchedResToData[wrd] = pathData
				}
			}
		}
	}

	return crsToTrigger
}

func GetJSONPath(path string, obj unstructured.Unstructured) ([]string, error) {
	expression, err := jsonpath.Compile(path)
	if err != nil {
		return nil, err
	}
	match, err := expression.Lookup(obj.Object)
	if err != nil {
		if strings.Contains(err.Error(), "key error") {
			return nil, nil
		}
		return nil, err
	}
	switch reflect.TypeOf(match).Kind() {
	case reflect.Slice:
		if res, ok := match.([]interface{}); !ok {
			return nil, errors.New("Error converting result to string")
		} else {
			strSlice := make([]string, 0)
			for _, element := range res {
				strSlice = append(strSlice, element.(string))
			}
			return strSlice, nil
		}
	case reflect.String:
		return []string{match.(string)}, nil
	}
	return nil, errors.New("Unsupported result")
}

func areEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}

	sort.Strings(s1)
	sort.Strings(s2)

	for idx := range s1 {
		if s1[idx] != s2[idx] {
			return false
		}
	}

	return true
}
