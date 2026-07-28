import React, { useEffect, useState } from "react";

interface DriftEvent {
  resource_id: string;
  resource_type: string;
  change_type: string;
  severity: string;
  actor: string;
  detected_at?: string;
}

const severityColor: Record<string, string> = {
  CRITICAL: "#e74c3c",
  HIGH: "#e67e22",
  MEDIUM: "#f1c40f",
  LOW: "#3498db",
  INFO: "#95a5a6",
};

export default function DriftFeed() {
  const [events, setEvents] = useState<DriftEvent[]>([]);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    const ws = new WebSocket("ws://localhost:8082/ws");

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onmessage = (msg) => {
      try {
        const event: DriftEvent = JSON.parse(msg.data);
        setEvents((prev) => [{ ...event, detected_at: new Date().toISOString() }, ...prev].slice(0, 50));
      } catch (e) {
        console.error("Failed to parse event", e);
      }
    };

    return () => ws.close();
  }, []);

  return (
    <div style={{ fontFamily: "monospace", padding: "20px", background: "#1a1a2e", color: "#eee", minHeight: "100vh" }}>
      <h1 style={{ color: "#4ec9b0" }}>
        InfraGuard — Live Drift Feed{" "}
        <span style={{ fontSize: "14px", color: connected ? "#2ecc71" : "#e74c3c" }}>
          ● {connected ? "CONNECTED" : "DISCONNECTED"}
        </span>
      </h1>
      <table style={{ width: "100%", borderCollapse: "collapse", marginTop: "20px" }}>
        <thead>
          <tr style={{ borderBottom: "2px solid #444", textAlign: "left" }}>
            <th style={{ padding: "8px" }}>Time</th>
            <th style={{ padding: "8px" }}>Resource</th>
            <th style={{ padding: "8px" }}>Change</th>
            <th style={{ padding: "8px" }}>Severity</th>
            <th style={{ padding: "8px" }}>Actor</th>
          </tr>
        </thead>
        <tbody>
          {events.length === 0 && (
            <tr><td colSpan={5} style={{ padding: "20px", textAlign: "center", color: "#666" }}>
              Waiting for drift events... (run the drift simulator to see live data)
            </td></tr>
          )}
          {events.map((e, i) => (
            <tr key={i} style={{ borderBottom: "1px solid #333" }}>
              <td style={{ padding: "8px" }}>{e.detected_at?.slice(11, 19)}</td>
              <td style={{ padding: "8px" }}>{e.resource_id}</td>
              <td style={{ padding: "8px" }}>{e.change_type}</td>
              <td style={{ padding: "8px" }}>
                <span style={{
                  background: severityColor[e.severity] || "#666",
                  padding: "2px 8px", borderRadius: "4px", color: "#fff", fontSize: "12px",
                }}>
                  {e.severity || "INFO"}
                </span>
              </td>
              <td style={{ padding: "8px", fontSize: "12px", color: "#999" }}>{e.actor}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
