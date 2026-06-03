"""Thin ClickHouse wrappers over the server operation API."""

from typing import Any

import pandas as pd

from ethpandaops import _runtime


def list_datasources() -> list[dict[str, Any]]:
    """List available ClickHouse datasources."""
    response = _runtime.invoke("clickhouse.list_datasources")
    data = response.get("data", {})
    datasources = data.get("datasources", [])
    if not isinstance(datasources, list):
        raise ValueError("Invalid clickhouse.list_datasources response shape")
    return datasources


def query(
    datasource: str,
    sql: str,
    parameters: dict[str, Any] | None = None,
) -> pd.DataFrame:
    """Execute a SQL query against a ClickHouse datasource."""
    return _runtime.invoke_tsv_dataframe(
        "clickhouse.query",
        {
            "datasource": datasource,
            "sql": sql,
            "parameters": parameters,
        },
    )


def query_raw(
    datasource: str,
    sql: str,
    parameters: dict[str, Any] | None = None,
) -> tuple[list[tuple], list[str]]:
    """Execute a SQL query and return raw rows plus column names."""
    return _runtime.invoke_tsv_rows(
        "clickhouse.query_raw",
        {
            "datasource": datasource,
            "sql": sql,
            "parameters": parameters,
        },
    )
