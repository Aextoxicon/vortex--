#!/usr/bin/env python3
"""
Vortex 测试数据生成脚本
创建测试用户、好友关系和示例消息
"""

import requests
import json
import time
from typing import Dict, List, Optional
from datetime import datetime

# 配置
BASE_URL = "http://localhost:8080"
PASSWORD = "Test123!"  # 符合密码要求：大小写字母、数字、特殊字符

# 测试用户数据
TEST_USERS = [
    {"username": "ABC123Test", "email": "abc123@example.com"},
    {"username": "XYZ789Test", "email": "xyz789@example.com"},
    {"username": "HelloWorld", "email": "hello@example.com"},
]

# 示例消息
SAMPLE_MESSAGES = [
    "你好！这是第一条测试消息 ABC123",
    "欢迎使用 Vortex！XYZ789",
    "今天天气真不错！",
    "Hello World! 测试消息",
    "这是一条很长很长的消息，用来测试消息长度限制，看看能不能正常发送和显示，应该没问题吧？",
]


class VortexTester:
    def __init__(self, base_url: str):
        self.base_url = base_url
        self.users: List[Dict] = []  # 存储创建的用户信息
        self.tokens: Dict[str, str] = {}  # 存储用户token

    def register_user(self, username: str, email: str, password: str) -> Optional[Dict]:
        """注册新用户"""
        url = f"{self.base_url}/api/auth/register"
        data = {
            "username": username,
            "password": password,
            "email": email
        }
        try:
            response = requests.post(url, json=data)
            if response.status_code == 201:
                result = response.json()
                print(f"✅ 成功注册用户：{username} (public_id: {result['public_id']})")
                # 注册时也返回 token，可以直接使用
                return {
                    "public_id": result["public_id"],
                    "username": result["username"],
                    "email": result["email"],
                    "token": result["token"]
                }
            else:
                print(f"❌ 注册用户 {username} 失败：{response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 注册用户 {username} 时出错：{e}")
            return None

    def login_user(self, username: str, password: str) -> Optional[str]:
        """用户登录获取token"""
        url = f"{self.base_url}/api/auth/login"
        data = {
            "username": username,
            "password": password
        }
        try:
            response = requests.post(url, json=data)
            if response.status_code == 200:
                result = response.json()
                token = result["token"]
                print(f"✅ 用户 {username} 登录成功")
                return token
            else:
                print(f"❌ 用户 {username} 登录失败: {response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 用户 {username} 登录时出错: {e}")
            return None

    def send_friend_request(self, sender_token: str, sender_public_id: str, target_public_id: str) -> Optional[int]:
        """发送好友请求"""
        url = f"{self.base_url}/api/friends/request/send/{target_public_id}"
        headers = {"Authorization": f"Bearer {sender_token}"}
        try:
            response = requests.post(url, headers=headers)
            if response.status_code == 201:
                result = response.json()
                request_id = result["id"]
                print(f"✅ 发送好友请求成功 (request_id: {request_id})")
                return request_id
            else:
                print(f"❌ 发送好友请求失败: {response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 发送好友请求时出错: {e}")
            return None

    def get_friend_requests(self, token: str) -> Optional[Dict]:
        """获取好友请求列表"""
        url = f"{self.base_url}/api/friends/requests"
        headers = {"Authorization": f"Bearer {token}"}
        try:
            response = requests.get(url, headers=headers)
            if response.status_code == 200:
                return response.json()
            else:
                print(f"❌ 获取好友请求失败: {response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 获取好友请求时出错: {e}")
            return None

    def accept_friend_request(self, token: str, request_id: int) -> bool:
        """接受好友请求"""
        url = f"{self.base_url}/api/friends/request/{request_id}/accept"
        headers = {"Authorization": f"Bearer {token}"}
        try:
            response = requests.post(url, headers=headers)
            if response.status_code == 200:
                print(f"✅ 接受好友请求成功 (request_id: {request_id})")
                return True
            else:
                print(f"❌ 接受好友请求失败: {response.status_code} - {response.text}")
                return False
        except Exception as e:
            print(f"❌ 接受好友请求时出错: {e}")
            return False

    def send_message(self, token: str, conv_id: str, content: str, retry: int = 3) -> Optional[Dict]:
        """发送消息（带重试）"""
        url = f"{self.base_url}/api/messages/send"
        headers = {"Authorization": f"Bearer {token}"}
        data = {
            "conv_id": conv_id,
            "content": content
        }
        
        for attempt in range(retry):
            try:
                response = requests.post(url, json=data, headers=headers)
                if response.status_code == 201:
                    result = response.json()
                    msg_id = result.get('message', {}).get('id')
                    print(f"✅ 发送消息成功 (msg_id: {msg_id})")
                    return result
                elif response.status_code == 500 and attempt < retry - 1:
                    # 数据库错误，等待后重试
                    print(f"⚠️  发送消息失败，重试 {attempt + 1}/{retry}: 500 - {response.text}")
                    time.sleep(1)
                else:
                    print(f"❌ 发送消息失败：{response.status_code} - {response.text}")
                    return None
            except Exception as e:
                if attempt < retry - 1:
                    print(f"⚠️  发送消息异常，重试 {attempt + 1}/{retry}: {e}")
                    time.sleep(1)
                else:
                    print(f"❌ 发送消息时出错：{e}")
                    return None
        
        return None

    def create_group(self, token: str, name: str, description: str = "") -> Optional[Dict]:
        """创建群组"""
        url = f"{self.base_url}/api/groups"
        headers = {"Authorization": f"Bearer {token}"}
        data = {
            "name": name,
            "description": description
        }
        try:
            response = requests.post(url, json=data, headers=headers)
            if response.status_code == 201:
                result = response.json()
                # API 直接返回 group_id, name, owner_public_id
                group = {
                    'id': result.get('group_id'),
                    'name': result.get('name'),
                    'owner_public_id': result.get('owner_public_id')
                }
                print(f"✅ 创建群组成功 (group_id: {group.get('id')})")
                return group
            else:
                print(f"❌ 创建群组失败：{response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 创建群组时出错：{e}")
            return None

    def join_group(self, token: str, group_id: str) -> bool:
        """加入群组"""
        url = f"{self.base_url}/api/groups/{group_id}/join"
        headers = {"Authorization": f"Bearer {token}"}
        try:
            response = requests.post(url, headers=headers)
            if response.status_code == 200:
                print(f"✅ 加入群组成功 ({group_id})")
                return True
            else:
                print(f"❌ 加入群组失败：{response.status_code} - {response.text}")
                return False
        except Exception as e:
            print(f"❌ 加入群组时出错：{e}")
            return False

    def check_messages(self, token: str) -> Optional[Dict]:
        """检查新消息"""
        url = f"{self.base_url}/api/check"
        headers = {"Authorization": f"Bearer {token}"}
        try:
            response = requests.get(url, headers=headers)
            if response.status_code == 200:
                result = response.json()
                print(f"✅ 检查新消息：has_new={result.get('has_new')}, unread_count={result.get('unread_count')}")
                return result
            else:
                print(f"❌ 检查新消息失败：{response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 检查新消息时出错：{e}")
            return None

    def get_conversations(self, token: str) -> Optional[Dict]:
        """获取会话列表"""
        url = f"{self.base_url}/api/conversations"
        headers = {"Authorization": f"Bearer {token}"}
        try:
            response = requests.get(url, headers=headers)
            if response.status_code == 200:
                result = response.json()
                conversations = result.get('conversations', [])
                print(f"✅ 获取会话列表成功 (共 {len(conversations)} 个会话)")
                return result
            else:
                print(f"❌ 获取会话列表失败：{response.status_code} - {response.text}")
                return None
        except Exception as e:
            print(f"❌ 获取会话列表时出错：{e}")
            return None

    def get_private_conv_id(self, public_id1: str, public_id2: str) -> str:
        """生成私聊会话ID"""
        if public_id1 < public_id2:
            return f"p_{public_id1}_{public_id2}"
        return f"p_{public_id2}_{public_id1}"

    def create_message_table_if_needed(self) -> bool:
        """确保消息表存在（等待 worker 创建）"""
        print("\n检查服务器状态...")
        try:
            response = requests.get(f"{self.base_url}/health")
            if response.status_code == 200:
                print("✅ 服务器运行正常")
                # Worker 启动时会自动创建消息表，等待一下
                print("等待 Worker 创建消息表...")
                time.sleep(3)
                return True
            else:
                print(f"❌ 健康检查失败：{response.status_code}")
                return False
        except Exception as e:
            print(f"❌ 无法连接到服务器：{e}")
            print("请确保服务器在运行")
            return False

    def setup(self):
        """设置测试环境"""
        print("\n" + "="*60)
        print("开始设置测试数据...")
        print("="*60)

        # 0. 检查服务器和消息表
        print("\n--- 0. 检查服务器状态 ---")
        if not self.create_message_table_if_needed():
            print("\n❌ 服务器未运行或消息表未创建，退出")
            return
        print("\n--- 1. 注册测试用户 ---")
        for user_data in TEST_USERS:
            result = self.register_user(user_data["username"], user_data["email"], PASSWORD)
            if result:
                self.users.append({
                    "username": user_data["username"],
                    "public_id": result["public_id"],
                    "email": user_data["email"]
                })
                # 使用注册时返回的 token
                self.tokens[user_data["username"]] = result["token"]
            time.sleep(0.5)  # 避免限流

        if len(self.users) < 2:
            print("\n❌ 需要至少 2 个用户来创建好友关系！")
            return

        # 2. 登录所有用户（重新获取 token，确保是最新的）
        print("\n--- 2. 登录用户 ---")
        for user in self.users:
            token = self.login_user(user["username"], PASSWORD)
            if token:
                self.tokens[user["username"]] = token
            time.sleep(0.5)

        # 3. 创建好友关系
        print("\n--- 3. 创建好友关系 ---")
        # 用户1 <-> 用户2
        self.create_friendship(0, 1)
        # 用户1 <-> 用户3
        self.create_friendship(0, 2)
        # 用户2 <-> 用户3
        self.create_friendship(1, 2)

        # 4. 发送示例消息
        print("\n--- 4. 发送示例消息 ---")
        self.send_sample_messages()

        # 5. 创建群组
        print("\n--- 5. 创建群组 ---")
        self.create_demo_group()

        # 6. 测试会话列表和新消息检查
        print("\n--- 6. 测试会话列表和新消息检查 ---")
        self.test_conversations_and_check()

        print("\n" + "="*60)
        print("🎉 测试数据设置完成！")
        print("="*60)
        self.print_summary()

    def create_friendship(self, user1_idx: int, user2_idx: int):
        """创建两个用户之间的好友关系"""
        user1 = self.users[user1_idx]
        user2 = self.users[user2_idx]
        token1 = self.tokens[user1["username"]]
        token2 = self.tokens[user2["username"]]

        print(f"\n创建好友关系: {user1['username']} <-> {user2['username']}")

        # 用户1 向 用户2 发送好友请求
        request_id = self.send_friend_request(token1, user1["public_id"], user2["public_id"])
        if not request_id:
            return
        time.sleep(0.5)

        # 用户2 查看收到的请求
        requests = self.get_friend_requests(token2)
        if requests and requests.get("received"):
            # 找到对应的请求并接受
            for req in requests["received"]:
                self.accept_friend_request(token2, req["id"])
                break
        time.sleep(0.5)

    def send_sample_messages(self):
        """发送示例消息"""
        # 用户 1 和 用户 2 之间的消息
        conv_id_1_2 = self.get_private_conv_id(self.users[0]["public_id"], self.users[1]["public_id"])
        print(f"\n在 {self.users[0]['username']} 和 {self.users[1]['username']} 之间发送消息:")
        self.send_message(self.tokens[self.users[0]["username"]], conv_id_1_2, SAMPLE_MESSAGES[0])
        time.sleep(0.5)
        self.send_message(self.tokens[self.users[1]["username"]], conv_id_1_2, SAMPLE_MESSAGES[1])
        time.sleep(0.5)
        self.send_message(self.tokens[self.users[0]["username"]], conv_id_1_2, SAMPLE_MESSAGES[2])
        time.sleep(0.5)

        # 用户 1 和 用户 3 之间的消息
        conv_id_1_3 = self.get_private_conv_id(self.users[0]["public_id"], self.users[2]["public_id"])
        print(f"\n在 {self.users[0]['username']} 和 {self.users[2]['username']} 之间发送消息:")
        self.send_message(self.tokens[self.users[2]["username"]], conv_id_1_3, SAMPLE_MESSAGES[3])
        time.sleep(0.5)
        self.send_message(self.tokens[self.users[0]["username"]], conv_id_1_3, SAMPLE_MESSAGES[4])
        time.sleep(0.5)

        # 用户 2 和 用户 3 之间的消息
        conv_id_2_3 = self.get_private_conv_id(self.users[1]["public_id"], self.users[2]["public_id"])
        print(f"\n在 {self.users[1]['username']} 和 {self.users[2]['username']} 之间发送消息:")
        self.send_message(self.tokens[self.users[1]["username"]], conv_id_2_3, "Hey! 这是来自 XYZ789Test 的消息！")
        time.sleep(0.5)

    def create_demo_group(self):
        """创建示例群组"""
        # 用户 1 创建群组
        group = self.create_group(
            self.tokens[self.users[0]["username"]],
            "测试群组",
            "这是一个用于测试的群组"
        )
        if group:
            group_id = group.get('id')
            # 其他用户加入群组
            time.sleep(0.5)
            self.join_group(self.tokens[self.users[1]["username"]], group_id)
            time.sleep(0.5)
            self.join_group(self.tokens[self.users[2]["username"]], group_id)
            time.sleep(0.5)

            # 在群组中发送消息
            # group_id 已经包含 "g_" 前缀，直接使用
            conv_id = group_id
            print(f"\n在群组中发送消息:")
            self.send_message(self.tokens[self.users[0]["username"]], conv_id, "欢迎大家加入测试群组！")
            time.sleep(0.5)
            self.send_message(self.tokens[self.users[1]["username"]], conv_id, "大家好！")
            time.sleep(0.5)

    def test_conversations_and_check(self):
        """测试会话列表和新消息检查"""
        # 获取用户 1 的会话列表
        print(f"\n获取 {self.users[0]['username']} 的会话列表:")
        conversations = self.get_conversations(self.tokens[self.users[0]["username"]])
        if conversations:
            for conv in conversations.get('conversations', []):
                conv_type = conv.get('type')
                name = conv.get('name')
                print(f"  - {conv_type}: {name}")

        # 检查新消息
        print(f"\n检查 {self.users[0]['username']} 的新消息:")
        self.check_messages(self.tokens[self.users[0]["username"]])

    def print_summary(self):
        """打印总结信息"""
        print("\n📋 测试数据总结:")
        print("-" * 40)
        print("用户列表:")
        for user in self.users:
            print(f"  - {user['username']}")
            print(f"    Public ID: {user['public_id']}")
            print(f"    Email: {user['email']}")
            print(f"    Password: {PASSWORD}")
        print("-" * 40)
        print("好友关系:")
        print(f"  - {self.users[0]['username']} <-> {self.users[1]['username']}")
        print(f"  - {self.users[0]['username']} <-> {self.users[2]['username']}")
        print(f"  - {self.users[1]['username']} <-> {self.users[2]['username']}")
        print("-" * 40)


def main():
    """主函数"""
    print("Vortex 测试数据生成脚本")
    print("="*60)

    tester = VortexTester(BASE_URL)
    tester.setup()


if __name__ == "__main__":
    main()