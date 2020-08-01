#!/usr/bin/env python

import json
import sys
import urllib.request as request


def txs_count(host, indexname, fromnum, lastnum):
    body = {
        "query": {
            "bool": {"must": [{"range": {"number": {"gte": fromnum, "lte": lastnum}}}]}
        },
        "size": 0,
        "aggs": {"sum": {"sum": {"field": "txs"}}},
    }

    url = "{}/{}/_search".format(host, indexname)

    req = request.Request(
        url=url,
        method="POST",
        data=json.dumps(body).encode("utf8"),
        headers={"content-type": "application/json"},
    )

    with request.urlopen(req) as f:
        data = f.read()
        objs = json.loads(data)

        txs = objs["aggregations"]["sum"]["value"]
        return txs

    return 0


def span_time(host, indexname, fromnum, lastnum):
    url0 = "{}/{}/_doc/block_{}".format(host, indexname, fromnum)
    url1 = "{}/{}/_doc/block_{}".format(host, indexname, lastnum)

    t0 = 0
    with request.urlopen(url0) as f:
        data = f.read()
        objs = json.loads(data)
        t0 = objs["_source"]["timestamp_ms"]

    t1 = 0
    with request.urlopen(url1) as f:
        data = f.read()
        objs = json.loads(data)
        t1 = objs["_source"]["timestamp_ms"]

    return t1 - t0


def tps(txs, spans):
    return txs / (spans / 1000)


def show(fromnum, lastnum, txs, spans, tps):
    print("{} - {:<8}  {:<18} {:<20} {:<20}".format(fromnum, lastnum, txs, spans/3600000, tps))


if __name__ == "__main__":
    if len(sys.argv) < 5:
        print("Usage: %s <url> <index_name> <start> <end>" % (sys.argv[0]))
        sys.exit(0)
    print("{:<15} {:<18} {:<20} {:<30}".format("Range", "Transactions", "Span", "TPS"))
    url = sys.argv[1]
    index_name = sys.argv[2]
    fromnum = int(sys.argv[3])
    lastnum = int(sys.argv[4])
    show(
        fromnum,
        lastnum,
        txs_count(url, index_name, fromnum, lastnum),
        span_time(url, index_name, fromnum, lastnum),
        tps(txs_count(url, index_name, fromnum, lastnum), span_time(url, index_name, fromnum, lastnum)),
    )
