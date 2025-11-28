request="POST"
url="http://127.0.0.1:9080/test"
header="Content-Type: application/json"
data='{"message": "hello"}'
echo "$request: $url"
echo "header: $header"
echo "body: $data"
curl --verbose --request "$request" --header "$header" --data "$data" "$url"
