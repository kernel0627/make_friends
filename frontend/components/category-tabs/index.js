Component({
  properties: {
    categories: { type: Array, value: [] },
    activeCategory: { type: String, value: '全部' },
    subCategories: { type: Array, value: [] },
    activeSubCategory: { type: String, value: '' },
  },
  methods: {
    onCategoryTap(e) {
      const category = e.currentTarget.dataset.category || ''
      this.triggerEvent('categorychange', { category })
    },
    onSubCategoryTap(e) {
      const subCategory = e.currentTarget.dataset.subCategory || ''
      this.triggerEvent('subcategorychange', { subCategory })
    },
  },
})