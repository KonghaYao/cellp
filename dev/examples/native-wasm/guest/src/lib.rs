wit_bindgen::generate!({
    world: "fixture",
    path: "wit",
    generate_all,
});

use cellp::kv::kv::{self, AccessError, KvError, Namespace};
use wstd::http::{Body, Method, Request, Response, StatusCode};

const KV_BINDING: &str = "VALUES";
const E2E_HOSTCALL_FAIL_KEY: &str = "__e2e_probe_hostcall_fail";

#[wstd::http_server]
async fn main(req: Request<Body>) -> Result<Response<Body>, wstd::http::Error> {
    let path = req.uri().path().to_string();
    let method = req.method().clone();

    if path == "/health" {
        let greeting = cellp::config::config::get("GREETING").unwrap_or_else(|| "hello-native".to_string());
        let body = format!("native-http-v1\n{greeting}\n");
        return Ok(Response::builder()
            .status(StatusCode::OK)
            .body(Body::from(body))?);
    }

    if let Some(probe) = path.strip_prefix("/probe/") {
        return handle_probe(probe, method, req).await;
    }

    let key = path.trim_start_matches('/').to_string();
    if key.is_empty() {
        return text_response(StatusCode::BAD_REQUEST, "Use /KEY.\n");
    }

    let ns = open_kv()?;

    match method {
        Method::PUT => {
            let mut body = req.into_body();
            let bytes = body.bytes_contents().await?;
            ns.put(&key, &bytes).map_err(kv_err)?;
            Ok(Response::builder()
                .status(StatusCode::NO_CONTENT)
                .body(Body::empty())?)
        }
        Method::DELETE => {
            ns.delete(&key).map_err(kv_err)?;
            Ok(Response::builder()
                .status(StatusCode::NO_CONTENT)
                .body(Body::empty())?)
        }
        Method::GET => match ns.get(&key).map_err(kv_err)? {
            Some(value) => Ok(Response::new(Body::from(value))),
            None => text_response(StatusCode::NOT_FOUND, "Not found.\n"),
        },
        _ => text_response(StatusCode::METHOD_NOT_ALLOWED, "Method not allowed.\n"),
    }
}

async fn handle_probe(
    probe: &str,
    method: Method,
    req: Request<Body>,
) -> Result<Response<Body>, wstd::http::Error> {
    match probe {
        "trap" => {
            let _ = method;
            let _ = req;
            core::arch::wasm32::unreachable();
        }
        "deadline" => {
            let _ = method;
            let _ = req;
            loop {
                core::hint::spin_loop();
            }
        }
        "memory" => {
            let _ = method;
            let _ = req;
            let mut chunks: Vec<Vec<u8>> = Vec::new();
            loop {
                chunks.push(vec![0_u8; 1024 * 1024]);
            }
        }
        "kv-hostcall-fail" => {
            if method != Method::PUT {
                return text_response(StatusCode::METHOD_NOT_ALLOWED, "Use PUT.\n");
            }
            let ns = open_kv()?;
            ns.put(E2E_HOSTCALL_FAIL_KEY, b"probe")
                .map_err(kv_err)?;
            Ok(Response::builder()
                .status(StatusCode::NO_CONTENT)
                .body(Body::empty())?)
        }
        "hostcall-flood" => {
            let _ = method;
            let _ = req;
            let ns = open_kv()?;
            for i in 0..16_u8 {
                let key = format!("flood-{i}");
                let _ = ns.get(&key).map_err(kv_err)?;
            }
            text_response(StatusCode::OK, "unexpected\n")
        }
        "kv-spin" => {
            let _ = method;
            let _ = req;
            let ns = open_kv()?;
            loop {
                let _ = ns.get("warm-key").map_err(kv_err)?;
            }
        }
        _ => text_response(StatusCode::NOT_FOUND, "Unknown probe.\n"),
    }
}

fn open_kv() -> Result<Namespace, wstd::http::Error> {
    match kv::open(KV_BINDING) {
        Ok(ns) => Ok(ns),
        Err(AccessError::Denied) => Err(wstd::http::Error::msg("kv binding denied")),
    }
}

fn kv_err(error: KvError) -> wstd::http::Error {
    wstd::http::Error::msg(format!("kv error: {error:?}"))
}

fn text_response(status: StatusCode, body: &str) -> Result<Response<Body>, wstd::http::Error> {
    Ok(Response::builder()
        .status(status)
        .body(Body::from(body.to_string()))?)
}
