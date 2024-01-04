/*
 * Copyright (c) 2022. TxnLab Inc.
 * All Rights reserved.
 */
package misc

import (
	"os"
	"strings"
)

var secretsMap = map[string]string{}

func SecretKeys() []string {
	var uniqKeys = map[string]bool{}
	for _, envVal := range os.Environ() {
		key := envVal[0:strings.IndexByte(envVal, '=')]
		uniqKeys[key] = true
	}
	for k, _ := range secretsMap {
		uniqKeys[k] = true
	}
	var retStrings []string
	for k, _ := range uniqKeys {
		retStrings = append(retStrings, k)
	}
	return retStrings
}

func GetSecret(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return secretsMap[key]
}

//func LoadCloudSecrets(ctx context.Context, log slog.Logger, credsFile string) error {
//	// Fetch the secrets defined for our particular service (if even there)
//	// GCP project in which to store secrets in Secret Manager.
//	projectID := os.Getenv("NFD_PROJECT_ID")
//	environment := GetEnvironment()
//	svcName := os.Getenv("CHART_NAME")
//
//	if svcName == "" {
//		log.Warnf("No chart name specified - skipping cloud secret load")
//		return nil
//	}
//
//	var (
//		client *secretmanager.Client
//		err    error
//	)
//	if credsFile == "" {
//		client, err = secretmanager.NewClient(ctx)
//	} else {
//		client, err = secretmanager.NewClient(ctx, option.WithCredentialsFile(credsFile))
//	}
//	if err != nil {
//		return fmt.Errorf("failed to setup client: %v", err)
//	}
//	defer client.Close()
//
//	log.Infof("project:%s env:%s svc:%s", projectID, environment, svcName)
//	// Build the request.
//	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
//		// Name: version.Name,
//		Name: fmt.Sprintf("projects/%s/secrets/%s-%s-secrets/versions/latest", projectID, environment, svcName),
//	}
//
//	// Call the API.
//	result, err := client.AccessSecretVersion(ctx, accessRequest)
//	if err != nil {
//		return fmt.Errorf("failed to access secret at:%s, error: %v", accessRequest.Name, err)
//	}
//
//	log.Infof("Was able to access secret payload, size:%d", len(result.Payload.Data))
//	return setSecretsAsEnvFileIntoMemory(result.Payload.Data)
//}
//
//// setSecretsAsEnvFileIntoMemory treats the passed in byte data as if it were an X=Y env file, parses it out and sets
//// into our package 'secretsMap' variable that our helper function uses to return secrets to internal code (but all
//// only in memory).
//func setSecretsAsEnvFileIntoMemory(envData []byte) error {
//	envMap, err := godotenv.Parse(bytes.NewReader(envData))
//	if err != nil {
//		return err
//	}
//
//	for key, value := range envMap {
//		secretsMap[strings.ToUpper(key)] = value
//	}
//	return nil
//}
