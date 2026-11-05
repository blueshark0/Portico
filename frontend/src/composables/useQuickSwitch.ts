import { ref } from 'vue'

// 快速切换面板的全局开关：工作台入口与全局 ⌘K/Ctrl+K 共用。
const quickSwitchOpen = ref(false)
const quickSwitchPlatform = ref('')

// 默认供应商变更计数：快捷切换保存成功后自增，供工作台等页面订阅刷新。
const profileRevision = ref(0)

export const useQuickSwitch = () => {
  const openQuickSwitch = (platform = '') => {
    quickSwitchPlatform.value = platform
    quickSwitchOpen.value = true
  }

  const closeQuickSwitch = () => {
    quickSwitchOpen.value = false
  }

  const notifyProfileChanged = () => {
    profileRevision.value += 1
  }

  return {
    quickSwitchOpen,
    quickSwitchPlatform,
    profileRevision,
    openQuickSwitch,
    closeQuickSwitch,
    notifyProfileChanged,
  }
}
