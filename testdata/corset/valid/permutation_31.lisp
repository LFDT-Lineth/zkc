;; Permutation between pairs of columns, where the second source column is
;; constant.
(defcolumns (X :i16) (Y :i16) (Z :i16) (C :i16 :constant 1) (one :i1 :constant 1))
(defpermutation test ((one X C)) ((one Y Z)))
