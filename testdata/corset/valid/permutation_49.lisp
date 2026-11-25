;; As permutation_20, but where only one target has a selector.
(defpermutation test ((_ s.X)) ((t1.one t1.Y) (_ t2.Z)))

(module t1)
(defcolumns (Y :i16) (one :i1 :constant 1))

(module t2)
(defcolumns (Z :i16))

(module s)
(defcolumns (X :i16))
