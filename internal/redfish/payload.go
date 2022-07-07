// Licensed to You under the Apache License, Version 2.0.

package redfish

import (
	"fmt"
	"log"
)

type RedfishPayload struct {
	Object map[string]interface{}
	Array  []interface{}
	Float  float64
	Client *RedfishClient
}

func (r *RedfishPayload) IsCollection() bool {
	_, ok := r.Object["Members"]
	return ok
}

func (r *RedfishPayload) IsEventCollection() bool {
        _, ok := r.Object["Events"]
        return ok
}

func (r *RedfishPayload) IsArray() bool {
	return r.Array != nil
}

func (r *RedfishPayload) GetCollectionSize() int {
	if r.Object["Members@odata.count"] == nil {
		return 0
	}
	return int(r.Object["Members@odata.count"].(float64))
}

func (r *RedfishPayload) GetEventSize() int {
        if r.Object["Events@odata.count"] == nil {
                return 0
        }
        return int(r.Object["Events@odata.count"].(float64))
}

func (r *RedfishPayload) GetArraySize() int {
	return len(r.Array)
}

func valueToPayload(client *RedfishClient, value interface{}) *RedfishPayload {
	ret := new(RedfishPayload)
	ret.Client = client
	switch v := value.(type) {
	case map[string]interface{}:
		ret.Object = v
	case []interface{}:
		ret.Array = v
	case float64:
		ret.Float = v
	default:
		log.Fatalf("Unknown type %T", v)
	}
	return ret
}

func (r *RedfishPayload) GetPropertyByName(name string) (*RedfishPayload, error) {
	value, ok := r.Object[name]
	if ok {
		uri := getUriFromValue(value)
		if len(uri) == 0 {
			return valueToPayload(r.Client, value), nil
		}
		return r.Client.GetUri(uri)
	}
	return nil, fmt.Errorf("No such element %s", name)
}

func (r *RedfishPayload) GetPropertyByIndex(index int) (*RedfishPayload, error) {
	if r.IsCollection() {
		value := r.Object["Members"]
		array := value.([]interface{})
		value = array[index]
		uri := getUriFromValue(value)
		if len(uri) == 0 {
			return valueToPayload(r.Client, value), nil
		}
		return r.Client.GetUri(uri)
	} else {
		if len(r.Array) >= index {
			value := r.Array[index]
			uri := getUriFromValue(value)
			if len(uri) == 0 {
				return valueToPayload(r.Client, value), nil
			}
			return r.Client.GetUri(uri)
		}
	}
	return nil, fmt.Errorf("No such element %d", index)
}

func (r *RedfishPayload) GetEventByIndex(index int) (*RedfishPayload, error) {
        if r.IsEventCollection() {
                value := r.Object["Events"]
                array := value.([]interface{})
                value = array[index]
                uri := getUriFromValue(value)
                if len(uri) == 0 {
                        return valueToPayload(r.Client, value), nil
                }
                return r.Client.GetUri(uri)
        } else {
                if len(r.Array) >= index {
                        value := r.Array[index]
                        uri := getUriFromValue(value)
                        if len(uri) == 0 {
                                return valueToPayload(r.Client, value), nil
                        }
                        return r.Client.GetUri(uri)
                }
        }
        return nil, fmt.Errorf("No such element %d", index)
}

func walkChild(value interface{}, client *RedfishClient, res *map[string]*RedfishPayload) {
	switch v := value.(type) {
	case map[string]interface{}:
		id, ok := v["@odata.id"]
		if ok {
			_, alreadyWalked := (*res)[id.(string)]
			if !alreadyWalked {
				log.Printf("Walking %s...\n", id)
				client.walkUri(id.(string), res)
			}
		} else {
			//There may be odata id's in child objects
			child := valueToPayload(client, v)
			child.walk(res)
		}
	case []interface{}:
		//There may be odata id's in child objects
		child := valueToPayload(client, v)
		child.walk(res)
	}
}

func (r *RedfishPayload) walk(res *map[string]*RedfishPayload) {
	for _, value := range r.Object {
		walkChild(value, r.Client, res)
	}
	for _, value := range r.Array {
		walkChild(value, r.Client, res)
	}
}

func getUriFromValue(value interface{}) string {
	switch value.(type) {
	case map[string]interface{}:
		break
	default:
		return ""
	}
	json := value.(map[string]interface{})
	if len(json) != 1 {
		return ""
	}
	odata, ok := json["@odata.id"]
	if !ok {
		return ""
	}
	return odata.(string)
}
