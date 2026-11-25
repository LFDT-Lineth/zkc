;; As permutation_31, but without a target selector.
(defcolumns (X :i16) (Y :i16) (Z :i16) (C :i16 :constant 1) (one :i1 :constant 1))
(defpermutation test ((one X C)) ((_ Y Z)))
