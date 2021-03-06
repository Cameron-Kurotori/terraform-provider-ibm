package ibm

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"time"

	"strings"

	kp "github.com/IBM/keyprotect-go-client"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceIBMKmskey() *schema.Resource {
	return &schema.Resource{
		Create:   resourceIBMKmsKeyCreate,
		Read:     resourceIBMKmsKeyRead,
		Update:   resourceIBMKmsKeyUpdate,
		Delete:   resourceIBMKmsKeyDelete,
		Exists:   resourceIBMKmsKeyExists,
		Importer: &schema.ResourceImporter{},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"instance_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Key protect or hpcs instance GUID",
			},
			"key_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Key ID",
			},
			"key_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Key name",
			},
			"type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "type of service hs-crypto or kms",
			},
			"endpoint_type": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateAllowedStringValue([]string{"public", "private"}),
				Description:  "public or private",
				ForceNew:     true,
				Default:      "public",
			},
			"standard_key": {
				Type:        schema.TypeBool,
				Default:     false,
				Optional:    true,
				ForceNew:    true,
				Description: "Standard key type",
			},
			"payload": {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				ForceNew: true,
			},
			"encrypted_nonce": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Only for imported root key",
			},
			"iv_value": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Only for imported root key",
			},
			"force_delete": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "set to true to force delete the key",
				ForceNew:    false,
				Default:     false,
			},
			"crn": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Crn of the key",
			},
			ResourceName: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The name of the resource",
			},

			ResourceCRN: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The crn of the resource",
			},

			ResourceStatus: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The status of the resource",
			},

			ResourceGroupName: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The resource group name in which resource is provisioned",
			},
			ResourceControllerURL: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The URL of the IBM Cloud dashboard that can be used to explore and view details about the resource",
			},
		},
	}
}

func resourceIBMKmsKeyCreate(d *schema.ResourceData, meta interface{}) error {
	kpAPI, err := meta.(ClientSession).keyManagementAPI()
	if err != nil {
		return err
	}

	hpcsEndpointApi, err := meta.(ClientSession).HpcsEndpointAPI()
	if err != nil {
		return err
	}

	rContollerClient, err := meta.(ClientSession).ResourceControllerAPIV2()
	if err != nil {
		return err
	}

	instanceID := d.Get("instance_id").(string)
	endpointType := d.Get("endpoint_type").(string)

	rContollerApi := rContollerClient.ResourceServiceInstanceV2()

	instanceData, err := rContollerApi.GetInstance(instanceID)
	if err != nil {
		return err
	}
	instanceCRN := instanceData.Crn.String()
	crnData := strings.Split(instanceCRN, ":")

	var hpcsEndpointURL string

	if crnData[4] == "hs-crypto" {
		resp, err := hpcsEndpointApi.Endpoint().GetAPIEndpoint(instanceID)
		if err != nil {
			return err
		}

		if endpointType == "public" {
			hpcsEndpointURL = "https://" + resp.Kms.Public + "/api/v2/keys"
		} else {
			hpcsEndpointURL = "https://" + resp.Kms.Private + "/api/v2/keys"
		}

		u, err := url.Parse(hpcsEndpointURL)
		if err != nil {
			return fmt.Errorf("Error Parsing hpcs EndpointURL")
		}
		kpAPI.URL = u
	} else if crnData[4] == "kms" {
		if endpointType == "private" {
			if !strings.HasPrefix(kpAPI.Config.BaseURL, "private") {
				kpAPI.Config.BaseURL = "private." + kpAPI.Config.BaseURL
			}
		}
	} else {
		return fmt.Errorf("Invalid or unsupported service Instance")
	}
	kpAPI.Config.InstanceID = instanceID
	name := d.Get("key_name").(string)
	standardKey := d.Get("standard_key").(bool)

	var keyCRN string
	if standardKey {
		if v, ok := d.GetOk("payload"); ok {
			//import standard key
			payload := v.(string)
			stkey, err := kpAPI.CreateImportedStandardKey(context.Background(), name, nil, payload)
			if err != nil {
				return fmt.Errorf(
					"Error while creating standard key with payload: %s", err)
			}
			log.Printf("New key created: %v", *stkey)
			keyCRN = stkey.CRN
		} else {
			//create standard key
			stkey, err := kpAPI.CreateStandardKey(context.Background(), name, nil)
			if err != nil {
				return fmt.Errorf(
					"Error while creating standard key: %s", err)
			}
			log.Printf("New key created: %v", *stkey)
			keyCRN = stkey.CRN
		}
		d.SetId(keyCRN)
	} else {
		if v, ok := d.GetOk("payload"); ok {
			payload := v.(string)
			encryptedNonce := d.Get("encrypted_nonce").(string)
			iv := d.Get("iv_value").(string)
			stkey, err := kpAPI.CreateImportedRootKey(context.Background(), name, nil, payload, encryptedNonce, iv)
			if err != nil {
				return fmt.Errorf(
					"Error while creating Root key with payload: %s", err)
			}
			log.Printf("New key created: %v", *stkey)
			keyCRN = stkey.CRN
		} else {
			stkey, err := kpAPI.CreateRootKey(context.Background(), name, nil)
			if err != nil {
				return fmt.Errorf(
					"Error while creating Root key: %s", err)
			}
			log.Printf("New key created: %v", *stkey)
			keyCRN = stkey.CRN
		}

		d.SetId(keyCRN)

	}

	return resourceIBMKmsKeyRead(d, meta)
}

func resourceIBMKmsKeyRead(d *schema.ResourceData, meta interface{}) error {
	kpAPI, err := meta.(ClientSession).keyManagementAPI()
	if err != nil {
		return err
	}
	crn := d.Id()
	crnData := strings.Split(crn, ":")
	endpointType := crnData[3]
	instanceID := crnData[len(crnData)-3]
	keyid := crnData[len(crnData)-1]

	hpcsEndpointApi, err := meta.(ClientSession).HpcsEndpointAPI()
	if err != nil {
		return err
	}

	var instanceType string
	var hpcsEndpointURL string

	if crnData[4] == "hs-crypto" {
		instanceType = "hs-crypto"
		resp, err := hpcsEndpointApi.Endpoint().GetAPIEndpoint(instanceID)
		if err != nil {
			return err
		}

		if endpointType == "public" {
			hpcsEndpointURL = "https://" + resp.Kms.Public + "/api/v2/keys"
		} else {
			hpcsEndpointURL = "https://" + resp.Kms.Private + "/api/v2/keys"
		}

		u, err := url.Parse(hpcsEndpointURL)
		if err != nil {
			return fmt.Errorf("Error Parsing hpcs EndpointURL")

		}
		kpAPI.URL = u
	} else if crnData[4] == "kms" {
		instanceType = "kms"
		if endpointType == "private" {
			if !strings.HasPrefix(kpAPI.Config.BaseURL, "private") {
				kpAPI.Config.BaseURL = "private." + kpAPI.Config.BaseURL
			}
		}
	} else {
		return fmt.Errorf("Invalid or unsupported service Instance")
	}

	kpAPI.Config.InstanceID = instanceID
	// keyid := d.Id()
	key, err := kpAPI.GetKey(context.Background(), keyid)
	if err != nil {
		return fmt.Errorf(
			"Get Key failed with error: %s", err)
	}
	d.Set("key_id", keyid)
	d.Set("standard_key", key.Extractable)
	d.Set("payload", key.Payload)
	d.Set("encrypted_nonce", key.EncryptedNonce)
	d.Set("iv_value", key.IV)
	d.Set("key_name", key.Name)
	d.Set("crn", key.CRN)
	d.Set("endpoint_type", endpointType)
	d.Set("type", instanceType)
	d.Set("force_delete", d.Get("force_delete").(bool))
	d.Set(ResourceName, key.Name)
	d.Set(ResourceCRN, key.CRN)
	d.Set(ResourceStatus, key.State)
	rcontroller, err := getBaseController(meta)
	if err != nil {
		return err
	}
	id := key.ID
	crn1 := strings.TrimSuffix(key.CRN, ":key:"+id)

	d.Set(ResourceControllerURL, rcontroller+"/services/kms/"+url.QueryEscape(crn1)+"%3A%3A")

	return nil

}

func resourceIBMKmsKeyUpdate(d *schema.ResourceData, meta interface{}) error {

	if d.HasChange("force_delete") {
		d.Set("force_delete", d.Get("force_delete").(bool))
	}
	return resourceIBMKmsKeyRead(d, meta)

}

func resourceIBMKmsKeyDelete(d *schema.ResourceData, meta interface{}) error {
	kpAPI, err := meta.(ClientSession).keyManagementAPI()
	if err != nil {
		return err
	}
	crn := d.Id()
	crnData := strings.Split(crn, ":")
	endpointType := crnData[3]
	instanceID := crnData[len(crnData)-3]
	keyid := crnData[len(crnData)-1]
	kpAPI.Config.InstanceID = instanceID

	hpcsEndpointApi, err := meta.(ClientSession).HpcsEndpointAPI()
	if err != nil {
		return err
	}
	var hpcsEndpointURL string

	if crnData[4] == "hs-crypto" {
		resp, err := hpcsEndpointApi.Endpoint().GetAPIEndpoint(instanceID)
		if err != nil {
			return err
		}

		if endpointType == "public" {
			hpcsEndpointURL = "https://" + resp.Kms.Public + "/api/v2/keys"
		} else {
			hpcsEndpointURL = "https://" + resp.Kms.Private + "/api/v2/keys"
		}

		u, err := url.Parse(hpcsEndpointURL)
		if err != nil {
			return fmt.Errorf("Error Parsing hpcs EndpointURL")
		}
		kpAPI.URL = u
	} else if crnData[4] == "kms" {
		if endpointType == "private" {
			if !strings.HasPrefix(kpAPI.Config.BaseURL, "private") {
				kpAPI.Config.BaseURL = "private." + kpAPI.Config.BaseURL
			}
		}
	} else {
		return fmt.Errorf("Invalid or unsupported service Instance")
	}

	force := d.Get("force_delete").(bool)
	f := kp.ForceOpt{
		Force: force,
	}
	_, err1 := kpAPI.DeleteKey(context.Background(), keyid, kp.ReturnRepresentation, f)
	if err1 != nil {
		return fmt.Errorf(
			"Error while deleting: %s", err1)
	}
	d.SetId("")
	return nil

}

func resourceIBMKmsKeyExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	kpAPI, err := meta.(ClientSession).keyManagementAPI()
	if err != nil {
		return false, err
	}

	hpcsEndpointApi, err := meta.(ClientSession).HpcsEndpointAPI()
	if err != nil {
		return false, err
	}

	crn := d.Id()
	crnData := strings.Split(crn, ":")
	endpointType := crnData[3]
	instanceID := crnData[len(crnData)-3]
	keyid := crnData[len(crnData)-1]
	kpAPI.Config.InstanceID = instanceID

	var hpcsEndpointURL string

	if crnData[4] == "hs-crypto" {
		resp, err := hpcsEndpointApi.Endpoint().GetAPIEndpoint(instanceID)
		if err != nil {
			return false, err
		}

		if endpointType == "public" {
			hpcsEndpointURL = "https://" + resp.Kms.Public + "/api/v2/keys"
		} else {
			hpcsEndpointURL = "https://" + resp.Kms.Private + "/api/v2/keys"
		}

		u, err := url.Parse(hpcsEndpointURL)
		if err != nil {
			return false, fmt.Errorf("Error Parsing hpcs EndpointURL")

		}
		kpAPI.URL = u
	} else if crnData[4] == "kms" {
		if endpointType == "private" {
			if !strings.HasPrefix(kpAPI.Config.BaseURL, "private") {
				kpAPI.Config.BaseURL = "private." + kpAPI.Config.BaseURL
			}
		}
	} else {
		return false, fmt.Errorf("Invalid or unsupported service Instance")
	}

	_, err = kpAPI.GetKey(context.Background(), keyid)
	if err != nil {
		kpError := err.(*kp.Error)
		if kpError.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil

}
