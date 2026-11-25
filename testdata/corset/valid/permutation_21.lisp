;; Fragmented permutation with two targets (of two columns each), where the
;; source and each target are in different modules.
(defpermutation test ((s.one s.X1 s.X2)) ((t1.one t1.Y1 t1.Y2) (t2.one t2.Z1 t2.Z2)))

(module s)
(defcolumns (X1 :i16) (X2 :i16) (one :i1 :constant 1))

(module t1)
(defcolumns (Y1 :i16) (Y2 :i16) (one :i1 :constant 1))

(module t2)
(defcolumns (Z1 :i16) (Z2 :i16) (one :i1 :constant 1))
