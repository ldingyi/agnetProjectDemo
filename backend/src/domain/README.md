该层为纯业务领域层。包含纯业务实体，完全针对业务设计
用户可见聊天会话使用 chatthread 模型表达。它是传统业务里的会话列表和消息记录，只保存前端可展示的用户消息、agent 最终回复、标题和时间。
agent 的模型上下文、系统提示、工具过程和 trace 不属于 chatthread；这些由 agent 层自己的 session/context 模型管理。
