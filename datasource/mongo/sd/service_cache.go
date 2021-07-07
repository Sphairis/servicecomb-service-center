/*
* Licensed to the Apache Software Foundation (ASF) under one or more
* contributor license agreements.  See the NOTICE file distributed with
* this work for additional information regarding copyright ownership.
* The ASF licenses this file to You under the Apache License, Version 2.0
* (the "License"); you may not use this file except in compliance with
* the License.  You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package sd

import (
	"reflect"
	"strings"

	cmap "github.com/orcaman/concurrent-map"

	"github.com/apache/servicecomb-service-center/datasource/mongo/client/model"
	"github.com/apache/servicecomb-service-center/datasource/sdcommon"
	"go.mongodb.org/mongo-driver/bson"
)

type serviceStore struct {
	dirty bool
	// the key is documentID, is value is mongo document.
	concurrentMap cmap.ConcurrentMap
	// the key is generated by indexFuncs,the value is a set of documentID.
	indexSets IndexCache
}

func init() {
	RegisterCacher(service, newServiceStore)
	ServiceIndexCols = NewIndexCols()
	ServiceIndexCols.AddIndexFunc(ServiceIDIndex)
	ServiceIndexCols.AddIndexFunc(ServiceInfoIndex)
	ServiceIndexCols.AddIndexFunc(ServiceVersionIndex)
}

func newServiceStore() *MongoCacher {
	options := DefaultOptions().SetTable(service)
	cache := &serviceStore{
		dirty:         false,
		concurrentMap: cmap.New(),
		indexSets:     NewIndexCache(),
	}
	serviceUnmarshal := func(doc bson.Raw) (resource sdcommon.Resource) {
		docID := MongoDocument{}
		err := bson.Unmarshal(doc, &docID)
		if err != nil {
			return
		}
		service := model.Service{}
		err = bson.Unmarshal(doc, &service)
		if err != nil {
			return
		}
		resource.Value = service
		resource.Key = docID.ID.Hex()
		return
	}
	return NewMongoCacher(options, cache, serviceUnmarshal)
}

func (s *serviceStore) Name() string {
	return service
}

func (s *serviceStore) Size() int {
	return s.concurrentMap.Count()
}

func (s *serviceStore) Get(key string) interface{} {
	if v, exist := s.concurrentMap.Get(key); exist {
		return v
	}
	return nil
}

func (s *serviceStore) ForEach(iter func(k string, v interface{}) (next bool)) {
	for k, v := range s.concurrentMap.Items() {
		if !iter(k, v) {
			break
		}
	}
}

func (s *serviceStore) GetValue(index string) []interface{} {
	docs := s.indexSets.Get(index)
	res := make([]interface{}, 0, len(docs))
	for _, id := range docs {
		if doc, exist := s.concurrentMap.Get(id); exist {
			res = append(res, doc)
		}
	}
	return res
}

func (s *serviceStore) Dirty() bool {
	return s.dirty
}

func (s *serviceStore) MarkDirty() {
	s.dirty = true
}

func (s *serviceStore) Clear() {
	s.dirty = false
	s.concurrentMap.Clear()
	s.indexSets.Clear()
}

func (s *serviceStore) ProcessUpdate(event MongoEvent) {
	serviceData, ok := event.Value.(model.Service)
	if !ok {
		return
	}
	if serviceData.Service == nil {
		return
	}
	// set the document data.
	s.concurrentMap.Set(event.DocumentID, event.Value)
	for _, index := range ServiceIndexCols.GetIndexes(serviceData) {
		// set the index sets.
		s.indexSets.Put(index, event.DocumentID)
	}
}

func (s *serviceStore) ProcessDelete(event MongoEvent) {
	serviceData, ok := s.concurrentMap.Get(event.DocumentID)
	if !ok {
		return
	}
	serviceMongo := serviceData.(model.Service)
	if serviceMongo.Service == nil {
		return
	}
	s.concurrentMap.Remove(event.DocumentID)
	for _, index := range ServiceIndexCols.GetIndexes(serviceMongo) {
		s.indexSets.Delete(index, event.DocumentID)
	}
}

func (s *serviceStore) isValueNotUpdated(value interface{}, newValue interface{}) bool {
	newService, ok := newValue.(model.Service)
	if !ok {
		return true
	}
	oldService, ok := value.(model.Service)
	if !ok {
		return true
	}
	return reflect.DeepEqual(newService, oldService)
}

func ServiceIDIndex(data interface{}) string {
	svc := data.(model.Service)
	return strings.Join([]string{svc.Domain, svc.Project, svc.Service.ServiceId}, "/")
}

func ServiceInfoIndex(data interface{}) string {
	svc := data.(model.Service)
	return strings.Join([]string{svc.Domain, svc.Project, svc.Service.AppId, svc.Service.ServiceName, svc.Service.Version}, "/")
}

func ServiceVersionIndex(data interface{}) string {
	svc := data.(model.Service)
	return strings.Join([]string{svc.Domain, svc.Project, svc.Service.AppId, svc.Service.ServiceName}, "/")
}