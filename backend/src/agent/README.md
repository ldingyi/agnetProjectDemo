该层存放ai开发相关。
业务项目不过分进行底层的实现。统一使用eino框架进行开发，尽量使用adk包进行简化开发。如需特别定制，首先选择eino的组件进行开发。
agent文件下，存放定义好的agent，以及agent的封装
session 只负责 agent 上下文会话，保存模型下一轮运行需要读取的上下文文本。
用户可见的聊天会话、会话列表和消息展示模型属于 domain/chatthread，不放在 agent/session 中。
系统提示、工具 trace 和内部推理过程不作为用户聊天记录暴露。
workflow：在固定的业务场景中，很多需求并不需要agent来进行分析思考，很多问题其实是一个固定的流程，因此将workflow单独抽出，可以作为tools供agent使用
turn：agent的流程编排。
