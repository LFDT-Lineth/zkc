;; Permutation between two columns of the same module, where every row is
;; selected using a constant selector.
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
(defpermutation test ((one X)) ((one Y)))
