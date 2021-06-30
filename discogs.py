import json
import fastunit
import pika
import uuid


incomplete_data = {
    "year": 1977,
    "publishing": [
        {
            "name": "Harvest",
            # "catno": "3C 064-05249"
            "catno": "SHVL 804"
        }
    ],
    "title": "The Dark Side Of The Moon",
    "recording": {
        "actors": [
            {
                "name": "Pink Floyd",
                "roles": ["performer"]
            }
        ]
    }
}


class RPCClient(object):

    def __init__(self, rpc_queue):
        self.rpc_queue = rpc_queue

        self.connection = pika.BlockingConnection(
            pika.ConnectionParameters(host='localhost'))

        self.channel = self.connection.channel()

        result = self.channel.queue_declare(queue='', exclusive=True)
        self.callback_queue = result.method.queue

        self.channel.basic_consume(
            queue=self.callback_queue,
            on_message_callback=self._on_response,
            auto_ack=True)

    def close(self):
        self.channel.close()
        self.connection.close()

    def _on_response(self, ch, method, props, body):
        if self.corr_id == props.correlation_id:
            self.response = body

    def call(self, payload):
        self.response = None
        self.corr_id = str(uuid.uuid4())
        self.channel.basic_publish(
            exchange='',
            routing_key=self.rpc_queue,
            properties=pika.BasicProperties(
                reply_to=self.callback_queue,
                correlation_id=self.corr_id,
            ),
            body=json.dumps(payload))
        while self.response is None:
            self.connection.process_data_events()
        return self.response

    def info(self):
        return self.call({"cmd": "info", "params": {}})

    def ping(self):
        return self.call({"cmd": "ping", "params": {}})


class OnlineDBClient(RPCClient):
    def __init__(self, queue_name):
        super().__init__(queue_name)

    def search_by_release_id(self, id):
        return self.call({"cmd": "search", "params": {"release_id": id}})

    def search_by_release(self, release_data):
        return self.call(
            {"cmd": "search", "params": {"release": release_data}})


class DiscogsClient(OnlineDBClient):
    def __init__(self):
        super().__init__('discogs')


class TestDiscogs(fastunit.TestCase):
    def setUp(self):
        self.cl = DiscogsClient()

    def tearDown(self):
        self.cl.close()

    def test_ping(self):
        self.assertEqual(self.cl.ping(), b'')

    def test_info(self):
        resp = json.loads(self.cl.info())
        self.assertEqual(resp["Name"], "discogs")

    def test_release_by_id(self):
        resp = json.loads(self.cl.search_by_release_id(4139588))
        self.assertEqual(
            resp[0]["entity"]["title"].lower(),
            "the dark side of the moon")

    def test_search_by_release(self):
        resp = json.loads(self.cl.search_by_release(incomplete_data))
        self.assertEqual(
            resp[0]["entity"]["title"].lower(),
            "the dark side of the moon")


if __name__ == '__main__':
    fastunit.main()
