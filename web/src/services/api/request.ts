import axios, { type AxiosRequestConfig } from "axios";

export interface ApiEnvelope<T = unknown> {
  code: number;
  data: T;
  msg: string;
  reason: string;
}

export class ApiError extends Error {
  readonly code: number;
  readonly reason: string;
  readonly httpStatus: number;

  constructor(code: number, reason: string, message: string, httpStatus = 200) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.reason = reason;
    this.httpStatus = httpStatus;
  }
}

const client = axios.create({
  baseURL: "/api",
  withCredentials: true,
  timeout: 30000,
});

async function unwrap<T>(promise: Promise<{ status: number; data: ApiEnvelope<T> }>): Promise<T> {
  try {
    const res = await promise;
    const body = res.data;
    if (body && typeof body === "object" && "code" in body) {
      if (body.code !== 0) {
        throw new ApiError(body.code, body.reason || "UNKNOWN_ERROR", body.msg || "Request failed", res.status);
      }
      return body.data;
    }
    return body as unknown as T;
  } catch (err) {
    if (err instanceof ApiError) {
      throw err;
    }
    if (axios.isAxiosError(err) && err.response?.data) {
      const data = err.response.data as Partial<ApiEnvelope>;
      throw new ApiError(
        data.code ?? err.response.status,
        data.reason || "HTTP_ERROR",
        data.msg || err.message,
        err.response.status,
      );
    }
    throw err;
  }
}

export const http = {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(client.get<ApiEnvelope<T>>(url, config));
  },
  post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(client.post<ApiEnvelope<T>>(url, data, config));
  },
  put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(client.put<ApiEnvelope<T>>(url, data, config));
  },
  patch<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(client.patch<ApiEnvelope<T>>(url, data, config));
  },
  delete<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return unwrap<T>(client.delete<ApiEnvelope<T>>(url, config));
  },
};
