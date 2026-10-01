#!/bin/sh
set -eu

echo "Creating Jungle Gaming SQS queues..."

cat > /tmp/dlq-attributes.json <<'EOF'
{
  "FifoQueue": "true",
  "ContentBasedDeduplication": "false"
}
EOF

DLQ_URL="$(
    awslocal sqs create-queue \
        --queue-name wager-transactions-dlq.fifo \
        --attributes file:///tmp/dlq-attributes.json \
        --query QueueUrl \
        --output text
)"

DLQ_ARN="$(
    awslocal sqs get-queue-attributes \
        --queue-url "$DLQ_URL" \
        --attribute-names QueueArn \
        --query 'Attributes.QueueArn' \
        --output text
)"

cat > /tmp/transactions-attributes.json <<EOF
{
  "FifoQueue": "true",
  "ContentBasedDeduplication": "false",
  "VisibilityTimeout": "30",
  "RedrivePolicy": "{\"deadLetterTargetArn\":\"${DLQ_ARN}\",\"maxReceiveCount\":\"5\"}"
}
EOF

awslocal sqs create-queue \
    --queue-name wager-transactions.fifo \
    --attributes file:///tmp/transactions-attributes.json

cat > /tmp/events-attributes.json <<'EOF'
{
  "FifoQueue": "true",
  "ContentBasedDeduplication": "false"
}
EOF

awslocal sqs create-queue \
    --queue-name wager-events.fifo \
    --attributes file:///tmp/events-attributes.json

echo "Jungle Gaming SQS queues created."