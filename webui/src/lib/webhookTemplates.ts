import { JsonMap } from "./api";

type WebhookTemplate = {
  name: string;
  template: JsonMap;
};

type TemplateGroup = Record<string, WebhookTemplate>;

// feishuCardTemplate 构建飞书 interactive 卡片模板（schema 2.0），内容支持 {{占位符}} 替换
function feishuCardTemplate(title: string, color: string, content: string): JsonMap {
  return {
    msg_type: "interactive",
    card: {
      schema: "2.0",
      header: {
        title: { tag: "plain_text", content: title },
        template: color
      },
      body: {
        direction: "vertical",
        elements: [{ tag: "markdown", content }]
      }
    }
  };
}

const smsReceivedTemplates: TemplateGroup = {
  generic: {
    name: "通用格式",
    template: {
      event: "{{event_type}}",
      data: {
        content: "{{content}}",
        send_number: "{{send_number}}",
        receive_number: "{{receive_number}}",
        receive_time: "{{receive_time}}",
        sms_ids: "{{sms_ids}}",
        direction: "{{direction}}"
      },
      timestamp: "{{receive_time}}"
    }
  },
  wechat_work: {
    name: "企业微信机器人",
    template: {
      msgtype: "text",
      text: {
        content: "收到短信\n发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}"
      }
    }
  },
  feige: {
    name: "飞鸽传书",
    template: {
      title: "新短信通知",
      content: "发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}",
      timestamp: "{{receive_time}}"
    }
  },
  dingtalk: {
    name: "钉钉机器人",
    template: {
      msgtype: "text",
      text: {
        content: "【短信通知】\n发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}"
      }
    }
  },
  feishu: {
    name: "飞书机器人",
    template: {
      msg_type: "text",
      content: {
        text: "【短信通知】\n发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}"
      }
    }
  },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "新短信通知",
      "blue",
      "**发件人:** {{send_number}}\n**收件人:** {{receive_number}}\n**内容:** {{content}}\n**时间:** {{receive_time}}"
    )
  },
  discord: {
    name: "Discord",
    template: {
      content: "收到新短信",
      embeds: [
        {
          title: "短信详情",
          color: 5814783,
          fields: [
            { name: "发件人", value: "{{send_number}}", inline: true },
            { name: "收件人", value: "{{receive_number}}", inline: true },
            { name: "内容", value: "{{content}}" },
            { name: "时间", value: "{{receive_time}}", inline: true }
          ],
          timestamp: "{{receive_time}}"
        }
      ]
    }
  },
  slack: {
    name: "Slack",
    template: {
      text: "收到新短信",
      blocks: [
        { type: "header", text: { type: "plain_text", text: "短信通知" } },
        {
          type: "section",
          fields: [
            { type: "mrkdwn", text: "*发件人:*\n{{send_number}}" },
            { type: "mrkdwn", text: "*收件人:*\n{{receive_number}}" }
          ]
        },
        { type: "section", text: { type: "mrkdwn", text: "*内容:*\n{{content}}" } },
        { type: "section", text: { type: "mrkdwn", text: "*时间:* {{receive_time}}" } }
      ]
    }
  },
  telegram: {
    name: "Telegram Bot",
    template: {
      chat_id: "",
      text: "*新短信通知*\n\n发件人: `{{send_number}}`\n收件人: `{{receive_number}}`\n内容: {{content}}\n时间: {{receive_time}}",
      parse_mode: "Markdown"
    }
  }
};

const smsSentSuccessTemplates: TemplateGroup = {
  generic: {
    name: "通用格式",
    template: {
      event: "{{event_type}}",
      data: {
        content: "{{content}}",
        send_number: "{{send_number}}",
        receive_number: "{{receive_number}}",
        send_time: "{{receive_time}}",
        direction: "{{direction}}"
      },
      timestamp: "{{event_time}}"
    }
  },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "短信发送成功\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【短信发送成功】\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【短信发送成功】\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "短信发送成功",
      "green",
      "**收件人:** {{receive_number}}\n**内容:** {{content}}\n**时间:** {{receive_time}}"
    )
  },
  discord: { name: "Discord", template: { embeds: [{ title: "短信发送成功", color: 3066993, fields: [{ name: "收件人", value: "{{receive_number}}", inline: true }, { name: "内容", value: "{{content}}" }, { name: "时间", value: "{{receive_time}}", inline: true }] }] } },
  slack: { name: "Slack", template: { text: "短信发送成功", blocks: [{ type: "header", text: { type: "plain_text", text: "短信发送成功" } }, { type: "section", text: { type: "mrkdwn", text: "*收件人:* {{receive_number}}\n*内容:* {{content}}\n*时间:* {{receive_time}}" } }] } },
  telegram: { name: "Telegram Bot", template: { chat_id: "", text: "*短信发送成功*\n\n收件人: `{{receive_number}}`\n内容: {{content}}\n时间: {{receive_time}}", parse_mode: "Markdown" } }
};

const smsSentFailedTemplates: TemplateGroup = {
  generic: {
    name: "通用格式",
    template: {
      event: "{{event_type}}",
      data: {
        content: "{{content}}",
        send_number: "{{send_number}}",
        receive_number: "{{receive_number}}",
        error: "{{error}}"
      },
      timestamp: "{{event_time}}"
    }
  },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "短信发送失败\n收件人: {{receive_number}}\n内容: {{content}}\n原因: {{error}}\n时间: {{event_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【短信发送失败】\n收件人: {{receive_number}}\n内容: {{content}}\n原因: {{error}}\n时间: {{event_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【短信发送失败】\n收件人: {{receive_number}}\n内容: {{content}}\n原因: {{error}}\n时间: {{event_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "短信发送失败",
      "red",
      "**收件人:** {{receive_number}}\n**内容:** {{content}}\n**失败原因:** {{error}}\n**时间:** {{event_time}}"
    )
  },
  discord: { name: "Discord", template: { embeds: [{ title: "短信发送失败", color: 15158332, fields: [{ name: "收件人", value: "{{receive_number}}", inline: true }, { name: "内容", value: "{{content}}" }, { name: "失败原因", value: "{{error}}" }, { name: "时间", value: "{{event_time}}", inline: true }] }] } },
  slack: { name: "Slack", template: { text: "短信发送失败", blocks: [{ type: "header", text: { type: "plain_text", text: "短信发送失败" } }, { type: "section", text: { type: "mrkdwn", text: "*收件人:* {{receive_number}}\n*失败原因:* {{error}}\n*内容:* {{content}}\n*时间:* {{event_time}}" } }] } },
  telegram: { name: "Telegram Bot", template: { chat_id: "", text: "*短信发送失败*\n\n收件人: `{{receive_number}}`\n原因: {{error}}\n内容: {{content}}\n时间: {{event_time}}", parse_mode: "Markdown" } }
};

const alertTriggeredTemplates: TemplateGroup = {
  generic: {
    name: "通用格式",
    template: {
      event: "{{event_type}}",
      data: {
        type: "{{alert_type}}",
        severity: "{{alert_severity}}",
        status: "{{alert_status}}",
        source: "{{alert_source}}",
        message: "{{alert_message}}",
        detail: "{{alert_detail}}",
        count: "{{alert_count}}",
        first_seen: "{{alert_first_seen}}",
        last_seen: "{{alert_last_seen}}"
      },
      timestamp: "{{event_time}}"
    }
  },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "告警触发\n类型: {{alert_type}}\n级别: {{alert_severity}}\n来源: {{alert_source}}\n消息: {{alert_message}}\n详情: {{alert_detail}}\n时间: {{event_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【告警触发】\n类型: {{alert_type}}\n级别: {{alert_severity}}\n来源: {{alert_source}}\n消息: {{alert_message}}\n详情: {{alert_detail}}\n时间: {{event_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【告警触发】\n类型: {{alert_type}}\n级别: {{alert_severity}}\n来源: {{alert_source}}\n消息: {{alert_message}}\n详情: {{alert_detail}}\n时间: {{event_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "告警触发",
      "red",
      "**类型:** {{alert_type}}\n**级别:** {{alert_severity}}\n**来源:** {{alert_source}}\n**消息:** {{alert_message}}\n**详情:** {{alert_detail}}\n**时间:** {{event_time}}"
    )
  },
  discord: { name: "Discord", template: { embeds: [{ title: "告警触发", color: 16711680, fields: [{ name: "类型", value: "{{alert_type}}", inline: true }, { name: "级别", value: "{{alert_severity}}", inline: true }, { name: "来源", value: "{{alert_source}}", inline: true }, { name: "消息", value: "{{alert_message}}" }, { name: "详情", value: "{{alert_detail}}" }], timestamp: "{{event_time}}" }] } },
  slack: { name: "Slack", template: { text: "告警触发", blocks: [{ type: "header", text: { type: "plain_text", text: "告警通知" } }, { type: "section", text: { type: "mrkdwn", text: "*类型:* {{alert_type}}\n*级别:* {{alert_severity}}\n*来源:* {{alert_source}}\n*消息:* {{alert_message}}\n*详情:* {{alert_detail}}\n*时间:* {{event_time}}" } }] } },
  telegram: { name: "Telegram Bot", template: { chat_id: "", text: "*告警触发*\n\n类型: `{{alert_type}}`\n级别: `{{alert_severity}}`\n来源: `{{alert_source}}`\n消息: {{alert_message}}\n详情: {{alert_detail}}\n时间: {{event_time}}", parse_mode: "Markdown" } }
};

const alertResolvedTemplates: TemplateGroup = {
  generic: {
    name: "通用格式",
    template: {
      event: "{{event_type}}",
      data: {
        type: "{{alert_type}}",
        severity: "{{alert_severity}}",
        source: "{{alert_source}}",
        message: "{{alert_message}}",
        detail: "{{alert_detail}}",
        count: "{{alert_count}}",
        resolved_at: "{{alert_resolved_at}}"
      },
      timestamp: "{{event_time}}"
    }
  },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "告警恢复\n类型: {{alert_type}}\n级别: {{alert_severity}}\n来源: {{alert_source}}\n消息: {{alert_message}}\n恢复时间: {{alert_resolved_at}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【告警恢复】\n类型: {{alert_type}}\n级别: {{alert_severity}}\n来源: {{alert_source}}\n消息: {{alert_message}}\n恢复时间: {{alert_resolved_at}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【告警恢复】\n类型: {{alert_type}}\n级别: {{alert_severity}}\n来源: {{alert_source}}\n消息: {{alert_message}}\n恢复时间: {{alert_resolved_at}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "告警恢复",
      "green",
      "**类型:** {{alert_type}}\n**级别:** {{alert_severity}}\n**来源:** {{alert_source}}\n**消息:** {{alert_message}}\n**恢复时间:** {{alert_resolved_at}}"
    )
  },
  discord: { name: "Discord", template: { embeds: [{ title: "告警恢复", color: 3066993, fields: [{ name: "类型", value: "{{alert_type}}", inline: true }, { name: "级别", value: "{{alert_severity}}", inline: true }, { name: "来源", value: "{{alert_source}}", inline: true }, { name: "消息", value: "{{alert_message}}" }, { name: "恢复时间", value: "{{alert_resolved_at}}", inline: true }], timestamp: "{{event_time}}" }] } },
  slack: { name: "Slack", template: { text: "告警恢复", blocks: [{ type: "header", text: { type: "plain_text", text: "告警恢复" } }, { type: "section", text: { type: "mrkdwn", text: "*类型:* {{alert_type}}\n*来源:* {{alert_source}}\n*消息:* {{alert_message}}\n*恢复时间:* {{alert_resolved_at}}" } }] } },
  telegram: { name: "Telegram Bot", template: { chat_id: "", text: "*告警恢复*\n\n类型: `{{alert_type}}`\n来源: `{{alert_source}}`\n消息: {{alert_message}}\n恢复时间: {{alert_resolved_at}}", parse_mode: "Markdown" } }
};

const syncCompletedTemplates: TemplateGroup = {
  generic: { name: "通用格式", template: { event: "{{event_type}}", data: { sync_scope: "{{sync_scope}}", total_modems: "{{total_modems}}", success_count: "{{success_count}}", failed_count: "{{failed_count}}", total_count: "{{total_count}}", new_count: "{{new_count}}" }, timestamp: "{{event_time}}" } },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "短信同步完成\n范围: {{sync_scope}}\n设备: {{success_count}}/{{total_modems}} 台\n短信总数: {{total_count}} 条\n新增: {{new_count}} 条\n时间: {{event_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【短信同步完成】\n范围: {{sync_scope}}\n设备: {{success_count}}/{{total_modems}} 台\n短信总数: {{total_count}} 条\n新增: {{new_count}} 条\n时间: {{event_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【短信同步完成】\n范围: {{sync_scope}}\n设备: {{success_count}}/{{total_modems}} 台\n短信总数: {{total_count}} 条\n新增: {{new_count}} 条\n时间: {{event_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "短信同步完成",
      "blue",
      "**范围:** {{sync_scope}}\n**设备:** {{success_count}}/{{total_modems}} 台\n**短信总数:** {{total_count}} 条\n**新增:** {{new_count}} 条\n**时间:** {{event_time}}"
    )
  }
};

const updateCompletedTemplates: TemplateGroup = {
  generic: { name: "通用格式", template: { event: "{{event_type}}", data: { status: "{{status}}", current_version: "{{current_version}}", latest_version: "{{latest_version}}", restart: "{{restart}}" }, timestamp: "{{event_time}}" } },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "系统更新完成\n状态: {{status}}\n当前版本: {{current_version}}\n最新版本: {{latest_version}}\n需要重启: {{restart}}\n时间: {{event_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【系统更新完成】\n状态: {{status}}\n当前版本: {{current_version}}\n最新版本: {{latest_version}}\n需要重启: {{restart}}\n时间: {{event_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【系统更新完成】\n状态: {{status}}\n当前版本: {{current_version}}\n最新版本: {{latest_version}}\n需要重启: {{restart}}\n时间: {{event_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "系统更新完成",
      "blue",
      "**状态:** {{status}}\n**当前版本:** {{current_version}}\n**最新版本:** {{latest_version}}\n**需要重启:** {{restart}}\n**时间:** {{event_time}}"
    )
  }
};

const modemOnlineTemplates: TemplateGroup = {
  generic: { name: "通用格式", template: { event: "{{event_type}}", data: { modem_name: "{{modem_name}}" }, timestamp: "{{event_time}}" } },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "设备上线\n设备: {{modem_name}}\n时间: {{event_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【设备上线】\n设备: {{modem_name}}\n时间: {{event_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【设备上线】\n设备: {{modem_name}}\n时间: {{event_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate("设备上线", "green", "**设备:** {{modem_name}}\n**时间:** {{event_time}}")
  },
  telegram: { name: "Telegram Bot", template: { chat_id: "", text: "*设备上线*\n\n设备: `{{modem_name}}`\n时间: {{event_time}}", parse_mode: "Markdown" } }
};

const ipChangedTemplates: TemplateGroup = {
  generic: { name: "通用格式", template: { event: "{{event_type}}", data: { old_ips: "{{old_ips}}", new_ips: "{{new_ips}}", added_ips: "{{added_ips}}", removed_ips: "{{removed_ips}}" }, timestamp: "{{event_time}}" } },
  wechat_work: { name: "企业微信机器人", template: { msgtype: "text", text: { content: "IP 地址变化\n旧 IP: {{old_ips}}\n新 IP: {{new_ips}}\n新增: {{added_ips}}\n移除: {{removed_ips}}\n时间: {{event_time}}" } } },
  dingtalk: { name: "钉钉机器人", template: { msgtype: "text", text: { content: "【IP 地址变化】\n旧 IP: {{old_ips}}\n新 IP: {{new_ips}}\n新增: {{added_ips}}\n移除: {{removed_ips}}\n时间: {{event_time}}" } } },
  feishu: { name: "飞书机器人", template: { msg_type: "text", content: { text: "【IP 地址变化】\n旧 IP: {{old_ips}}\n新 IP: {{new_ips}}\n新增: {{added_ips}}\n移除: {{removed_ips}}\n时间: {{event_time}}" } } },
  feishu_card: {
    name: "飞书卡片",
    template: feishuCardTemplate(
      "IP 地址变化",
      "orange",
      "**旧 IP:** {{old_ips}}\n**新 IP:** {{new_ips}}\n**新增:** {{added_ips}}\n**移除:** {{removed_ips}}\n**时间:** {{event_time}}"
    )
  },
  telegram: { name: "Telegram Bot", template: { chat_id: "", text: "*IP 地址变化*\n\n旧 IP: `{{old_ips}}`\n新 IP: `{{new_ips}}`\n新增: {{added_ips}}\n移除: {{removed_ips}}\n时间: {{event_time}}", parse_mode: "Markdown" } }
};

export const PRESET_TEMPLATES_BY_EVENT: Record<string, TemplateGroup> = {
  sms_received: smsReceivedTemplates,
  sms_sent_success: smsSentSuccessTemplates,
  sms_sent_failed: smsSentFailedTemplates,
  alert_triggered: alertTriggeredTemplates,
  alert_resolved: alertResolvedTemplates,
  sms_sync_completed: syncCompletedTemplates,
  system_update_completed: updateCompletedTemplates,
  modem_online: modemOnlineTemplates,
  ip_changed: ipChangedTemplates,
  all: {
    generic: {
      name: "通用格式",
      template: {
        event: "{{event_type}}",
        timestamp: "{{event_time}}"
      }
    }
  }
};

export const EVENT_VARIABLE_HINTS: Record<string, string> = {
  sms_received: "{{event_type}}, {{event_time}}, {{content}}, {{send_number}}, {{receive_number}}, {{receive_time}}, {{direction}}, {{device_imei}}, {{sim_iccid}}, {{sim_imsi}}, {{operator}}, {{modem_name}}, {{sms_ids}}",
  sms_sent_success: "{{event_type}}, {{event_time}}, {{content}}, {{send_number}}, {{receive_number}}, {{receive_time}}, {{direction}}, {{device_imei}}, {{sim_iccid}}, {{sim_imsi}}, {{operator}}, {{modem_name}}",
  sms_sent_failed: "{{event_type}}, {{event_time}}, {{content}}, {{send_number}}, {{receive_number}}, {{receive_time}}, {{direction}}, {{device_imei}}, {{sim_iccid}}, {{sim_imsi}}, {{operator}}, {{modem_name}}, {{error}}",
  alert_triggered: "{{event_type}}, {{event_time}}, {{alert_type}}, {{alert_severity}}, {{alert_status}}, {{alert_source}}, {{alert_message}}, {{alert_detail}}, {{alert_count}}, {{alert_first_seen}}, {{alert_last_seen}}",
  alert_resolved: "{{event_type}}, {{event_time}}, {{alert_type}}, {{alert_severity}}, {{alert_status}}, {{alert_source}}, {{alert_message}}, {{alert_detail}}, {{alert_count}}, {{alert_resolved_at}}",
  sms_sync_completed: "{{event_type}}, {{event_time}}, {{sync_scope}}, {{total_modems}}, {{success_count}}, {{failed_count}}, {{total_count}}, {{new_count}}",
  system_update_completed: "{{event_type}}, {{event_time}}, {{status}}, {{current_version}}, {{latest_version}}, {{restart}}",
  modem_online: "{{event_type}}, {{event_time}}, {{modem_name}}",
  ip_changed: "{{event_type}}, {{event_time}}, {{old_ips}}, {{new_ips}}, {{added_ips}}, {{removed_ips}}",
  all: "变量取决于触发事件类型，建议使用默认格式或按具体事件配置"
};

export const PRESET_TEMPLATES = smsReceivedTemplates;
