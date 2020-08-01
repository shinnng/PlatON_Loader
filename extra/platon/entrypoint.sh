#!/usr/bin/env bash

set -e
set -v

PLATON=/usr/local/bin/platon
PLATON_HOME=/data/platon

if [ "${NEW_ACCOUNT}" = "true" ]; then
	echo "123456" >/tmp/password
	${PLATON} --datadir ${PLATON_HOME}/data account new --password /tmp/password
	rm /tmp/password
fi

if [ "${INIT}" = "true" ]; then
	${PLATON} --verbosity 4 --datadir ${PLATON_HOME}/data init ${PLATON_HOME}/genesis.json
fi

DEBUG=
if [ "${ENABLE_DEBUG}" = "true" ]; then
	DEBUG=--debug
fi

PPROF=
if [ "${ENABLE_PPROF}" = "true" ]; then
	PPROF="--pprof --pprofaddr 0.0.0.0 --pprofport ${PPROFPORT}"
fi

WS=
if [ "${ENABLE_WS}" = "true" ]; then
	WS="--ws --wsaddr ${HOST} --wsport ${WSPORT} --wsapi ${WSAPI}"
fi

RPC=
if [ "${ENABLE_RPC}" = "true" ]; then
	RPC="--rpc --rpcaddr ${HOST} --rpcport ${RPCPORT} --rpcapi ${RPCAPI}"
fi

BOOT=
if [ "${BOOTNODES}" != "" ]; then
	BOOT="--bootnodesv4 ${BOOTNODES}"
fi

DISCOVER=--nodiscover
if [ "${ENABLE_DISCOVER}" = "true" ]; then
	DISCOVER=
fi

V5DISC=
if [ "${ENABLE_V5DISC}" = "true" ]; then
	V5DISC=--v5disc
fi

SYNCMODE_ARG=
if [ "${SYNCMODE}" != "" ]; then
	SYNCMODE_ARG="--syncmode ${SYNCMODE}"
fi

LIGHT_SRV=
if [ "${ENABLE_LIGHT_SRV}" = "true" ]; then
	LIGHT_SRV="--lightserv 10"
	SYNCMODE_ARG=
fi

DBGCFLAG=
if [ "${DISABLEDBGC}" = "true" ]; then
	DBGCFLAG="--db.nogc"
fi

DBGCMPTFLAG=
if [ "${DBGCMPT}" = "true" ]; then
	DBGCMPTFLAG="--db.gc_mpt"
fi

#-txpool.globaltxcount 1000 --txpool.lifetime 120s \

${PLATON} --identity platon --datadir ${PLATON_HOME}/data \
	--nodekey ${PLATON_HOME}/data/nodekey \
	--cbft.blskey ${PLATON_HOME}/data/blskey \
	--port ${P2PPORT} ${DEBUG} --verbosity ${VERBOSITY} \
	${PPROF} ${WS} ${RPC} \
	--metrics --ipcdisable --txpool.nolocals \
	${BOOT} ${DISCOVER} ${V5DISC} ${TRACING} \
	--maxpeers ${MAXPEERS} --maxconsensuspeers ${MAXCONSENSUSPEERS} \
	${SYNCMODE_ARG} ${LIGHT_SRV} --wsorigins \* \
	-txpool.globaltxcount ${TXCOUNT} --txpool.lifetime 120s \
	--txpool.accountslots 128 \
	--txpool.globalslots 6000 \
	--db.gc_interval ${DBGCINTERVAL} \
	--db.gc_timeout ${DBGCTIMEOUT} ${DBGCFLAG} ${DBGCMPTFLAG} \
    --cache.triedb ${CACHE_TRIEDB}
