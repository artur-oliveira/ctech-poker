# KMS policy: SecureStrings, DynamoDB, S3

The KMS bill must stay ~0 (free tier 20k requests/month). An SSM SecureString read with
decryption and every request to a table on the `aws/dynamodb` key is a billed KMS request.

1. **Decrypt at boot / cold start, cache.** The API reads its SecureStrings in `start.sh`
   at service start. The `reconcile` and `tablecleanup` Lambdas resolve them once in
   `main()` (cold start), never in the handler. A new loop or handler must follow the
   same rule; if it truly needs rotation without restart, read without decryption (ciphertext
   + Version, no KMS call) and decrypt only on a Version change, or use a TTL of 1h or more
   and state the rotation latency.
2. **DynamoDB uses the AWS-owned key**: `TableEncryptionV2.dynamoOwnedKey()` in
   `cdk/lib/dynamodb-stack.ts`, asserted for every table by `cdk/test/dynamodb-stack.test.ts`.
   Switching existing tables is an in-place SSESpecification update (`cdk diff`: no replacement).
3. **S3 is SSE-S3** (`S3_MANAGED` in `storage-stack`, `archiver-stack`), never `aws:kms`.
