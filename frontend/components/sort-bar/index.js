Component({
  properties: {
    sortBy: { type: String, value: 'hot' },
  },
  data: {
    options: [
      { key: 'hot', label: '推荐' },
      { key: 'latest', label: '最新' },
      { key: 'nearby', label: '附近' },
    ],
  },
  methods: {
    onSortTap(e) {
      const sortBy = e.currentTarget.dataset.sortBy || 'hot'
      this.triggerEvent('sortchange', { sortBy })
    },
  },
})