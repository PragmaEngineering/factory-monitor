<script lang="ts">
  import { Send, ArrowRight, Bot, Users, MessageSquare } from "@lucide/svelte";
  import type { Message, Session, SessionMessage } from "../lib/types";
  import { api } from "../lib/api";
  import { time, short } from "../lib/format";
  let {
    messages,
    sessions,
    executionID,
    disabled,
    refresh,
    drafts = $bindable({}),
    initialRecipient = "all",
  }: {
    messages: Message[];
    sessions: Session[];
    executionID: string;
    disabled: boolean;
    refresh: () => void;
    drafts: Record<string, string>;
    initialRecipient?: string;
  } = $props();
  let recipient = $state("all");
  $effect(() => {
    recipient = initialRecipient;
  });
  let sending = $state(false);
  let error = $state("");
  let deliveries = $state<SessionMessage[]>([]);
  let expanded = $state<string | null>(null);
  const key = $derived(executionID + ":" + recipient);
  const session = $derived(sessions.find((s) => s.id === recipient));
  const filtered = $derived(
    messages.filter(
      (m) =>
        recipient === "all" ||
        (recipient === "operator"
          ? m.recipient_names?.includes("operator") ||
            m.sender_name === "operator"
          : m.sender_name === session?.name ||
            m.recipient_names?.includes(session?.name || "")),
    ),
  );
  const traffic = $derived.by(() => {
    const lanes = new Map<
      string,
      { from: string; to: string; count: number }
    >();
    for (const message of messages)
      for (const to of message.recipient_names ?? message.recipients) {
        const from = message.sender_name || short(message.sender);
        const key = from + "→" + to;
        const lane = lanes.get(key) ?? { from, to, count: 0 };
        lane.count++;
        lanes.set(key, lane);
      }
    return [...lanes.values()];
  });
  $effect(() => {
    messages;
    const id = recipient;
    const controller = new AbortController();
    deliveries = [];
    if (id !== "all" && id !== "operator")
      api
        .sessionMessages(id, controller.signal)
        .then((x) => (deliveries = x))
        .catch((e) => {
          if (!controller.signal.aborted) error = e.message;
        });
    return () => controller.abort();
  });
  async function send() {
    const target = recipient,
      draftKey = key,
      body = (drafts[key] || "").trim();
    if (!body || !session) return;
    sending = true;
    error = "";
    try {
      await api.send(target, body);
      drafts[draftKey] = "";
      deliveries = await api.sessionMessages(target);
      refresh();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      sending = false;
    }
  }
</script>

<div class="messages-page">
  <div class="traffic-panel">
    <div class="section-heading">
      <span><ArrowRight size={16} />Observed communication</span><small
        >{messages.length} persisted messages</small
      >
    </div>
    <div class="traffic-lanes">
      {#each traffic as lane}<div class="traffic-lane">
          <span>{lane.from}</span>
          <div class="traffic-wire">
            <i style={`width:${Math.min(100, 10 + lane.count * 3)}%`}
            ></i><ArrowRight size={14} />
          </div>
          <span>{lane.to}</span><b>{lane.count}</b>
        </div>{:else}<p class="muted">No communication recorded.</p>{/each}
    </div>
  </div>
  <div class="conversation-layout">
    <aside class="participant-index">
      <h3>Inboxes</h3>
      <button
        class:chosen={recipient === "all"}
        onclick={() => (recipient = "all")}
        ><Users size={16} />All participants</button
      >
      <button
        class:chosen={recipient === "operator"}
        onclick={() => (recipient = "operator")}
        ><MessageSquare size={16} />Operator</button
      >
      {#each sessions.filter((s) => s.runner !== "hook") as member}<button
          class:chosen={recipient === member.id}
          onclick={() => (recipient = member.id)}
          ><Bot size={16} /><span
            >{member.name}<small>{short(member.id)} · {member.state}</small
            ></span
          ></button
        >{/each}
    </aside>
    <section class="conversation">
      <div class="section-heading">
        <span
          >{session?.name ||
            (recipient === "operator"
              ? "Operator inbox"
              : "All messages")}</span
        ><small>{filtered.length} messages</small>
      </div>
      <div class="message-list">
        {#each filtered as message (message.id)}<article class="message-card">
            <header>
              <strong>{message.sender_name || short(message.sender)}</strong
              ><ArrowRight size={12} /><span
                >{message.recipient_names?.join(", ") ||
                  message.recipients.map(short).join(", ")}</span
              ><time>{time(message.at)}</time>
            </header>
            <div class="message-kind">{message.payload.kind}</div>
            {#each message.payload.parts as part}{#if part.kind === "text"}<p>
                  {String(part.text)}
                </p>{:else}<pre
                  class:collapsed={expanded !== message.id}>{JSON.stringify(
                    part.kind === "data" ? part.data : part,
                    null,
                    2,
                  )}</pre>{/if}{/each}
            <button
              class="text-button"
              onclick={() =>
                (expanded = expanded === message.id ? null : message.id)}
              >{expanded === message.id
                ? "Collapse"
                : "Expand full message"}</button
            >
          </article>{:else}<div class="empty-state">
            <MessageSquare size={26} />
            <h3>No messages in this view</h3>
            <p>Messages will appear as participants communicate.</p>
          </div>{/each}
        {#each deliveries as message (message.id)}<article
            class="message-card operator-message"
          >
            <header>
              <strong>Operator</strong><ArrowRight size={12} /><span
                >{session?.name}</span
              ><time>{time(message.created_at)}</time>
            </header>
            <p>{message.body}</p>
            <small
              >{message.state} · delivery state does not prove the agent acted</small
            >
          </article>{/each}
      </div>
      {#if session}<form
          class="composer"
          onsubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          <label for="message">Message {session.name}</label><textarea
            id="message"
            placeholder="Send guidance to this agent…"
            value={drafts[key] || ""}
            oninput={(e) => (drafts[key] = e.currentTarget.value)}
            rows="3"
            maxlength="16384"></textarea>
          <div>
            <small
              >Sent to this session only. Formal decisions are answered in
              Decisions.</small
            ><button
              class="primary"
              disabled={disabled ||
                sending ||
                session.state !== "running" ||
                !drafts[key]?.trim()}
              ><Send size={14} />{sending ? "Sending…" : "Send message"}</button
            >
          </div>
          {#if error}<p class="error-text" role="alert">{error}</p>{/if}
        </form>{:else}<div class="composer-hint">
          Select an agent session to send a message.
        </div>{/if}
    </section>
  </div>
</div>
