#!/usr/bin/env python

import json
import sys
import urllib.request as request
import plotly.graph_objects as go
import pandas as pd


def min_timestamp(url):
    body = {
        "query": {"bool": {"must": [{"range": {"number": {"gte": 1}}}]}},
        "size": 0,
        "aggs": {"min_t": {"min": {"field": "timestamp_ms"}}},
    }

    req = request.Request(
        url=url,
        method="POST",
        data=json.dumps(body).encode("utf8"),
        headers={"content-type": "application/json"},
    )

    with request.urlopen(req) as f:
        data = f.read()
        objs = json.loads(data)

        t = objs["aggregations"]["min_t"]["value"]
        return t
    return 0


def max_timestamp(url):
    body = {
        "query": {"bool": {"must": [{"range": {"number": {"gte": 1}}}]}},
        "size": 0,
        "aggs": {"max_t": {"max": {"field": "timestamp_ms"}}},
    }

    req = request.Request(
        url=url,
        method="POST",
        data=json.dumps(body).encode("utf8"),
        headers={"content-type": "application/json"},
    )

    with request.urlopen(req) as f:
        data = f.read()
        objs = json.loads(data)

        t = objs["aggregations"]["max_t"]["value"]
        return t
    return 0


def count_txs(url, tfrom, tend):
    body = {
        "size": 0,
        "aggs": {
            "interval": {
                "date_range": {
                    "field": "timestamp_ms",
                    "ranges": [{"from": tfrom, "to": tend}],
                },
                "aggs": {"sum": {"sum": {"field": "txs"}}},
            }
        },
    }

    req = request.Request(
        url=url,
        method="POST",
        data=json.dumps(body).encode("utf8"),
        headers={"content-type": "application/json"},
    )

    with request.urlopen(req) as f:
        data = f.read()
        objs = json.loads(data)

        buckets = objs["aggregations"]["interval"]["buckets"]
        return buckets[0]["sum"]["value"]
    return 0


def run(host, indexname, mov, output):
    url = "{}/{}/_search".format(host, indexname)

    min_t = min_timestamp(url)
    max_t = max_timestamp(url)

    if min_t == 0 or max_t == 0 or max_t < min_t:
        print("invalid time: min: %d max: %d\n" % (min_t, max_t))
        return

    step = 1
    interval = step * 1000
    begin = int(min_t / 1000) * 1000
    # end = begin + 9 * interval
    end = begin + mov * interval
    #sstep = 1
    c = 0
    l = []
    x = []
    y = []
    while end < max_t:
        count = count_txs(url, begin, end)
        l.append((c * step, count, count / mov))
        x.append(c * step)
        y.append(count / step)
        begin = begin + interval
        end = end + interval
        c = c + 1

    # draw(x, y, "./test.png")

    fp = open(output, "wb")
    fp.write("sequence,txs,avg_txs\n".encode("utf8"))
    for (idx, count, avg) in l:
        s = "{},{},{}\n".format(idx, count, avg)
        fp.write(s.encode("utf8"))
    fp.close()

    #draw(img_output, output)


def draw(img_file, csv_file):
    data = pd.read_csv(csv_file)
    fig = go.Figure(
        data=go.Scatter(x=data["sequence"], y=data["avg_txs"], mode="lines")
    )
    fig.update_layout()
    fig.write_image(img_file, width=1600, height=700, scale=1.0)


if __name__ == "__main__":
    if len(sys.argv) < 5:
        print("{} <url> <indexname> <mov> <output csv>".format(sys.argv[0]))
        sys.exit(0)
    run(sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4])
