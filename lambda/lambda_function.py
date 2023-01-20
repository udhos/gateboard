import json
import boto3

sqs = boto3.client('sqs')
queue_url = 'https://sqs.us-east-1.amazonaws.com/140330866198/gateboard'
secret = 'secret'

def forbidden():
    return {
        "statusCode": 403,
        "body": "forbidden"
    }
    
def lambda_handler(event, context):
    
    event_str = json.dumps(event)
    
    print(event_str)
    
    headers = event['headers']
    
    auth = headers.get('authorization')
    if auth is None:
        print("missing header authorization")
        return forbidden()
    
    fields = auth.split(None, 1)
    if len(fields) < 2:
        print("missing token in header authorization")
        return forbidden()
    
    token = fields[1]
    if token != secret:
        print("invalid token in header authorization")
        return forbidden()
    
    body = event['body']
    
    response = sqs.send_message(
        QueueUrl=queue_url,
        MessageAttributes={},
        MessageBody=body
    )

    return {
        "statusCode": 200,
        "body": "ok"
    }
