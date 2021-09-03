package thousandeyes

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
 	"unicode"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/william20111/go-thousandeyes"
)

func expandAgents(v interface{}) thousandeyes.Agents {
	var agents thousandeyes.Agents

	for _, er := range v.([]interface{}) {
		rer := er.(map[string]interface{})
		agent := &thousandeyes.Agent{
			AgentID: rer["agent_id"].(int),
		}
		agents = append(agents, *agent)
	}

	return agents
}

func expandAlertRules(v interface{}) thousandeyes.AlertRules {
	var alertRules thousandeyes.AlertRules

	for _, er := range v.([]interface{}) {
		rer := er.(map[string]interface{})
		alertRule := &thousandeyes.AlertRule{
			RuleID: rer["rule_id"].(int),
		}
		alertRules = append(alertRules, *alertRule)
	}

	return alertRules
}

func expandBGPMonitors(v interface{}) thousandeyes.BGPMonitors {
	var bgpMonitors thousandeyes.BGPMonitors

	for _, er := range v.([]interface{}) {
		rer := er.(map[string]interface{})
		bgpMonitor := &thousandeyes.BGPMonitor{
			MonitorID: rer["monitor_id"].(int),
		}
		bgpMonitors = append(bgpMonitors, *bgpMonitor)
	}

	return bgpMonitors
}

func expandDNSServers(v interface{}) []thousandeyes.Server {
	var dnsServers []thousandeyes.Server

	for _, er := range v.([]interface{}) {
		rer := er.(map[string]interface{})
		targetDNSServer := &thousandeyes.Server{
			ServerName: rer["server_name"].(string),
		}
		dnsServers = append(dnsServers, *targetDNSServer)
	}

	return dnsServers
}

func unpackSIPAuthData(i interface{}) thousandeyes.SIPAuthData {
	var m = i.(map[string]interface{})
	var sipAuthData = thousandeyes.SIPAuthData{}

	for k, v := range m {
		if k == "auth_user" {
			sipAuthData.AuthUser = v.(string)
		}
		if k == "password" {
			sipAuthData.Password = v.(string)
		}
		if k == "port" {
			port, err := strconv.Atoi(v.(string))
			if err == nil {
				sipAuthData.Port = port
			}
		}
		if k == "protocol" {
			sipAuthData.Protocol = v.(string)
		}
		if k == "sip_proxy" {
			sipAuthData.SIPProxy = v.(string)
		}
		if k == "sip_registrar" {
			sipAuthData.SIPRegistrar = v.(string)
		}
		if k == "user" {
			sipAuthData.User = v.(string)
		}
	}

	return sipAuthData
}

// ResourceBuildStruct fills the struct at a given address by querying a
// schema.ResourceData object for the matching field.  It discovers the
// matching value name by getting the JSON key from the struct field,
// and then fills in the value according to the struct field's type.
func ResourceBuildStruct(d *schema.ResourceData, structPtr interface{}) interface{} {
	v := reflect.ValueOf(structPtr).Elem()
	t := reflect.TypeOf(v.Interface())
	for i := 0; i < v.NumField(); i++ {
		tag := GetJSONKey(t.Field(i))
		tfName := CamelCaseToUnderscore(tag)
		val, ok := d.GetOk(tfName)
		if ok {
			newVal := FillValue(val, v.Field(i).Interface())
			setVal := reflect.ValueOf(newVal)
			v.Field(i).Set(setVal)
		}
	}
	return structPtr
}

// ResourceRead sets values for a schema.ResourceData object by names derived
// from the fields of the struct at the provided pointer.
func ResourceRead(d *schema.ResourceData, structPtr interface{}) error {
	v := reflect.ValueOf(structPtr).Elem()
	t := reflect.TypeOf(v.Interface())
	for i := 0; i < v.NumField(); i++ {
		tag := GetJSONKey(t.Field(i))
		tfName := CamelCaseToUnderscore(tag)
		val, err := ReadValue(v.Field(i).Interface())
		if err != nil {
			return err
		}
		val, err = FixReadValues(val, tfName)
		if err != nil {
			return err
		}
		err = d.Set(tfName, val)
		if err != nil {
			return err
		}
	}
	return nil
}

// FixReadValues adjusts certain values returned from ThousandEyes to make them
// processable by this Terraform plugin.  This includes removing extraneous
// information that ThousandEyes returns when querying certain resources (ie,
// when querying a group it may return a list of associated tests with details)
// and transforms certain values to match the expected schema.
// We need to account for this data on so that it does not get saved to state and
// cause conflict with configuration.
func FixReadValues(m interface{}, name string) (interface{}, error) {
	switch name {
	// Remove all fields from agent definitions except for agent ID.
	case "agents":
		for i, v := range m.([]interface{}) {
			agent := v.(map[string]interface{})
			m.([]interface{})[i] = map[string]interface{}{
				"agent_id": agent["agent_id"],
			}
		}

	// Remove all alert rule fields except for rule ID.
	case "alert_rules":
		for i, v := range m.([]interface{}) {
			rule := v.(map[string]interface{})
			m.([]interface{})[i] = map[string]interface{}{
				"rule_id": rule["rule_id"],
			}
		}

	// Remove all public BGP monitors. (ThousandEyes does not allow
	// specifying individual public BGP monitors, and all available
	// public BGP monitors are returned if public BGP monitors are enabled.)
	case "bgp_monitors":
		monitors := m.([]interface{})
		// Edit the monitors slice in place, to return the same type.
		i := 0
		for i < len(monitors) {
			monitor := monitors[i].(map[string]interface{})
			if monitor["monitor_type"] == "Public" {
				// Remove this item from the slice
				monitors = append(monitors[:i], monitors[i+1:]...)
			} else {
				monitors[i] = map[string]interface{}{
					"monitor_id": monitor["monitor_id"],
				}
				i = i + 1
			}
		}
		m = monitors

	// Remove all dns_server fields except for the server name.
	case "dns_servers":
		for i, v := range m.([]interface{}) {
			servers := v.(map[string]interface{})
			m.([]interface{})[i] = map[string]interface{}{
				"server_name": servers["server_name"],
			}
		}

	// Remove all group fields except for the group ID.
	case "groups":
		for i, v := range m.([]interface{}) {
			group := v.(map[string]interface{})
			m.([]interface{})[i] = map[string]interface{}{
				"group_id": group["group_id"],
			}
		}

	// custom_headers is currently unsupported due to complications with Terraform
	// and the object schema.  It will presently be removed from state, and when
	// a solution is found it will be transformed here according to the specification
	// of that solution.
	case "custom_headers":
		m = nil

	// download_limit may appear as a string instead of an integer.
	case "download_limit":
		var err error
		if reflect.TypeOf(m) == reflect.TypeOf("") {
			if m.(string) == "" {
				m = 0
			} else {
				m, err = strconv.Atoi(m.(string))
				if err != nil {
					return nil, err
				}
			}
		}

	// Remove the owning account from the list of shared accounts.
	case "shared_with_accounts":
		accounts := m.([]interface{})
		if account_group_id == 0 {
			if len(accounts) > 1 {
				return nil, errors.New("Resources are shared between account groups, but account_group_id is not set.")
			}
			// A single listed account should be the owning account group.
			if len(accounts) == 1 {
				return nil, nil
			}
		}
		i := 0
		for i < len(accounts) {
			account := accounts[i].(map[string]interface{})
			//  Compare to account group ID stored in global variable.
			shared_aid := account["aid"].(int)
			if shared_aid == account_group_id {
				// Remove this item from the slice
				accounts = append(accounts[:i], accounts[i+1:]...)
			} else {
				accounts[i] = map[string]interface{}{
					"aid": shared_aid,
				}
				i = i + 1
			}
		}
		m = accounts

	// target_sip_credentials is presented as a map by ThousandEyes, but
	// limitations in Terraform's type system require us to declare its schema
	// as a single-item list in order to represent the map with values of
	// mixed types.
	case "target_sip_credentials":
		m = []interface{}{
			m.(map[string]interface{}),
		}

  case "notifications":
    var e interface{}
    var err error
    // this is a special case to handle internal email structure inside the notifications block
    e, err = FixReadValues(m.(map[string]interface{})["email"].(map[string]interface{}), "email")
    if err != nil {
      return nil, err
    }
    m.(map[string]interface{})["email"] = e

    m = []interface{}{
      m.(map[string]interface{}),
    }

  case "email":
    m = []interface{}{
      m.(map[string]interface{}),
    }

	// Remove tests.
	case "tests":
		m = nil
	}

	return m, nil
}

// ReadValue returns a value with key names for which Terraform will be able to
// identify in the Schema.  This is required because calling the Set function on
// a struct results in the JSON tag name (instead of the Terraform config key)
// being used for schema lookups.
func ReadValue(structPtr interface{}) (interface{}, error) {
	var err error
	v := reflect.Indirect(reflect.ValueOf(structPtr))
	t := reflect.TypeOf(v.Interface())
	switch t.Kind() {
	case reflect.Struct:
		// For structs, return a map with key names set to be translations of
		// the JSON key names.
		newMap := make(map[string]interface{})
		for i := 0; i < v.NumField(); i++ {
			tag := GetJSONKey(t.Field(i))
			tfName := CamelCaseToUnderscore(tag)
			newMap[tfName], err = ReadValue(v.Field(i).Interface())
		}
		if err != nil {
			return nil, err
		}
		return newMap, nil
	case reflect.Slice:
		// If it's a list, create an empty version of
		// that collection type, and then recurse for each child value (passing the
		// extended key name).
		var newSlice []interface{}
		for i := 0; i < v.Len(); i++ {
			newVal, err := ReadValue(v.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			newSlice = append(newSlice, newVal)
		}
		return newSlice, nil

	default:
		return structPtr, nil
	}
}

// ResourceUpdate updates values of a struct for the provided pointer if
// matching changes for those values are found in a provided
// schema.ResourceData object.
func ResourceUpdate(d *schema.ResourceData, structPtr interface{}) interface{} {
	d.Partial(true)
	v := reflect.ValueOf(structPtr).Elem()
	t := reflect.TypeOf(v.Interface())
	for i := 0; i < v.NumField(); i++ {
		tag := GetJSONKey(t.Field(i))
		tfName := CamelCaseToUnderscore(tag)
		if d.HasChange(tfName) {
			newVal := FillValue(d.Get(tfName), v.Field(i).Interface())
			setVal := reflect.ValueOf(newVal)
			v.Field(i).Set(setVal)
		}
	}
	d.Partial(false)
	return structPtr
}

// ResourceSchemaBuild creates a map of schemas based on the fields
// of the provided struct.
func ResourceSchemaBuild(referenceStruct interface{}, schemas map[string]*schema.Schema) map[string]*schema.Schema {
	newSchema := map[string]*schema.Schema{}
	v := reflect.ValueOf(referenceStruct)
	t := reflect.TypeOf(referenceStruct)

	for i := 0; i < v.NumField(); i++ {
		tag := GetJSONKey(t.Field(i))
		tfName := CamelCaseToUnderscore(tag)
		if val, ok := schemas[tfName]; ok {
			newSchema[tfName] = val
		}
	}
	return newSchema
}

// FillValue takes a value from the Terraform resource data and translates it
// to the correct type, based on the type of the target parameter.
func FillValue(source interface{}, target interface{}) interface{} {
	// We determine how to interpret the supplied value based on
	// the type of the target argument.
	vt := reflect.ValueOf(target)
	switch vt.Kind() {
	case reflect.Slice:
		// When the target is a slice, we create a new slice of the same type,
		// then recurse with the value of each element.
		vs := reflect.ValueOf(source)
		tt := reflect.TypeOf(target)
		tte := reflect.TypeOf(target).Elem() // The type of items in the slice
		ntte := reflect.New(tte).Elem()
		newSlice := reflect.New(tt).Elem()
		for i := 0; i < vs.Len(); i++ {
			toAppend := FillValue(vs.Index(i).Interface(), ntte.Interface())
			appendVal := reflect.ValueOf(toAppend)
			newSlice = reflect.Append(newSlice, appendVal)
		}
		return newSlice.Interface()
	case reflect.Struct:
		// When the target is a struct, we assume that the source is a map
		// containing values corresponding to the struct's fields, then
		// recurse on each value looked up to get the value to be set.

		// Due to limitations of Terraform's schema handling, some maps may
		// be delivered inside single-item slices.  This occurs when maps
		// must be declared as lists of terraform resources, whether to
		// define specific key names or to have values of mixed types,
		// neither of which is supported by Terraform's implementation of
		// maps.
		vs := reflect.ValueOf(source)
		structSource := source
		if vs.Kind() == reflect.Slice {
			structSource = source.([]interface{})[0]
    } else if vs.Kind() == reflect.Ptr {
      structSource = source.(*schema.Set).List()
      if len(structSource.([]interface{})) != 0 {
        structSource = structSource.([]interface{})[0]
      } else {
        source = nil
      }
    }
		t := reflect.TypeOf(vt.Interface())
		newStruct := reflect.New(t).Interface()
		setStruct := reflect.ValueOf(newStruct).Elem()
		if source != nil {
			m := structSource.(map[string]interface{})
			for i := 0; i < vt.NumField(); i++ {
				tag := GetJSONKey(t.Field(i))
				tfName := CamelCaseToUnderscore(tag)
				if mv, ok := m[tfName]; ok {
					newVal := FillValue(mv, vt.Field(i).Interface())
					setStruct.Field(i).Set(reflect.ValueOf(newVal))
				}
			}
		}
		return setStruct.Interface()
	case reflect.Int:
		// Values destined to be ints may come to us as strings.
		if reflect.TypeOf(source).Kind() == reflect.String {
			i, _ := strconv.Atoi(source.(string))
			return i
		}

		return source
	default:
		// If we haven't matched one of the above cases, then there
		// is likely no reason to translate.
		return source
	}
}

// UnderscoreToLowerCamelCase translates from words separated by
// underscores to camel case with initial lowercase.
// ie, a_string would become aString
func UnderscoreToLowerCamelCase(s string) string {
	// We have a map of exceptions to the usual conversion logic.
	exceptions := map[string]string{
		"ip_addresses": "IPAddresses",
	}
	if val, ok := exceptions[s]; ok {
		return val
	}
	s = strings.ToLower(s)
	s = strings.Replace(s, "_", " ", -1)
	s = strings.Title(s)
	s = strings.Replace(s, " ", "", -1)
	firstChar := string(s[0])
	s = strings.Replace(s, firstChar, strings.ToLower(firstChar), 1)
	return s
}

// CamelCaseToUnderscore translates from camel case (with any leading case)
// to underscore separated words.
// ie, either aString and AString would become a_string
func CamelCaseToUnderscore(s string) string {
	input := []rune(s)
	output := []rune{}
	for i, r := range input {
		if unicode.IsUpper(r) {
			if i != 0 && i < len(input)-1 {
				if unicode.IsLower(input[i+1]) {
					output = append(output, []rune("_")[0])
				}
			}
			output = append(output, unicode.ToLower(r))
		} else {
			output = append(output, r)
		}
	}
	return string(output)
}

// GetJSONKey returns the JSON object key for the struct which is represented
// by the passed reflect.StructField instance.
func GetJSONKey(v reflect.StructField) string {
	s := v.Tag.Get("json")
	return strings.Split(s, ",")[0]
}
